package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"unicode/utf8"

	"campfire/internal/shared/model"
)

// Follow relays Docker's demultiplexed stdout and stderr with bounded chunks.
func (Docker) Follow(ctx context.Context, name string, emit func(string) error) error {
	command := exec.CommandContext(ctx, "docker", "logs", "--follow", "--tail", "200", name)
	return followCommand(ctx, command, emit)
}

func followCommand(ctx context.Context, command *exec.Cmd, emit func(string) error) error {
	reader, writer := io.Pipe()
	command.Stdout = writer

	command.Stderr = writer
	if err := command.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()

		return errors.New("Docker console unavailable")
	}

	done := make(chan error, 1)

	go func() { err := command.Wait(); _ = writer.CloseWithError(err); done <- err }()

	stop := context.AfterFunc(ctx, func() { _ = reader.CloseWithError(ctx.Err()) })
	defer stop()
	defer func() { _ = reader.Close() }()

	buffer := make([]byte, 8192)
	pending := []byte{}

	var result error

	for {
		n, err := reader.Read(buffer)
		pending = append(pending, buffer[:n]...)

		end := len(pending)
		if err == nil {
			for end > 0 && !utf8.Valid(pending[:end]) && len(pending)-end < utf8.UTFMax {
				end--
			}
		}

		if end > 0 {
			if emitErr := emit(string(pending[:end])); emitErr != nil {
				result = emitErr
				break
			}

			pending = append(pending[:0], pending[end:]...)
		}

		if err != nil {
			if !errors.Is(err, io.EOF) {
				result = errors.New("Docker console interrupted")
			}

			break
		}
	}

	if result != nil {
		_ = command.Process.Kill()
		_ = reader.Close()
	}

	if err := <-done; err != nil && result == nil {
		result = errors.New("Docker console interrupted")
	}

	return result
}

// StreamConsole follows the current game or installer without holding lifecycle locks.
func (m *Manager) StreamConsole(ctx context.Context, d model.Deployment, emit func(string) error) error {
	if !safeID.MatchString(d.ServerID) {
		return errors.New("invalid server identity")
	}

	state, err := m.load(d.ServerID)
	if err != nil {
		return err
	}

	if !state.mu.TryLock() {
		return errors.New("runtime busy; reconnect console")
	}
	installed, busy, operation := state.Installed, state.Status.Busy, state.Status.Operation
	state.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stop := context.AfterFunc(m.ctx, cancel)
	defer stop()

	name := container(d.ServerID)

	if !installed || busy && operation == "install" {
		if !busy || operation != "install" {
			file, err := os.Open(filepath.Join(m.root(d.ServerID), "installer.log"))
			if err == nil {
				defer func() { _ = file.Close() }()

				data, err := io.ReadAll(io.LimitReader(file, 65536))
				if err != nil {
					return errors.New("installer output unavailable")
				}

				return emit(string(data))
			}
		}

		name += "-install"
	}

	provider, ok := m.Docker.(interface {
		Follow(context.Context, string, func(string) error) error
	})
	if !ok {
		return errors.New("console streaming unavailable")
	}

	return provider.Follow(ctx, name, emit)
}
