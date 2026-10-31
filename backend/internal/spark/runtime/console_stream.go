package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

const consoleContainerPollInterval = 250 * time.Millisecond
const consoleInspectTimeout = 3 * time.Second
const consolePreloadLines = "1000"

// Follow relays Docker's demultiplexed stdout and stderr with bounded chunks.
func (Docker) Follow(ctx context.Context, name string, emit func(string) error) error {
	command, err := dockerCommand(ctx, "logs", "--timestamps", "--follow", "--tail", consolePreloadLines, name)
	if err != nil {
		return err
	}

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
	if !utils.IsValidResourceID(d.ServerID) {
		return errors.New("invalid server identity")
	}

	state, err := m.load(d.ServerID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stop := context.AfterFunc(m.ctx, cancel)
	defer stop()

	provider, ok := m.Docker.(interface {
		Follow(context.Context, string, func(string) error) error
	})
	if !ok {
		return errors.New("console streaming unavailable")
	}

	ticker := time.NewTicker(consoleContainerPollInterval)
	defer ticker.Stop()

	for {
		if state.mu.TryLock() {
			installed, busy, operation, failure := state.Installed, state.Status.Busy, state.Status.Operation, state.Status.Error
			state.mu.Unlock()

			name := container(d.ServerID)

			installer := !installed || operation == "install" || operation == "update" || operation == "reinstall"
			if installer {
				name += "-install"

				if !busy {
					file, err := os.Open(filepath.Join(m.root(d.ServerID), "installer.log"))
					if err == nil {
						defer func() { _ = file.Close() }()

						data, err := io.ReadAll(io.LimitReader(file, 65536))
						if err != nil {
							return errors.New("installer output unavailable")
						}

						return emit(string(data))
					}

					if !errors.Is(err, os.ErrNotExist) {
						return errors.New("installer output unavailable")
					}

					if failure != "" {
						return emit(failure + "\n")
					}

					return nil
				}
			}

			check, cancel := context.WithTimeout(ctx, consoleInspectTimeout)
			output, err := m.Docker.Run(check, nil, "ps", "--all", "--filter", "name=^/"+name+"$", "--format", "{{.ID}}")

			cancel()

			if err != nil {
				return err
			}

			if strings.TrimSpace(output) != "" {
				return provider.Follow(ctx, name, emit)
			}

			if !busy {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
