package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"campfire/internal/shared/buildinfo"
	"campfire/internal/shared/protocol"
	"campfire/internal/shared/sparkrelease"
)

const updateHeartbeatInterval = 10 * time.Second
const updateVersionCheckTimeout = 10 * time.Second

// RestartError requests a clean runtime shutdown followed by executable replacement.
type RestartError struct{ Executable string }

func (e *RestartError) Error() string { return "Spark update installed; restarting agent" }

func updateWithHeartbeats(ctx context.Context, execute func() protocol.CommandResponse, send func(protocol.Message) error) protocol.CommandResponse {
	ctx, cancel := context.WithCancel(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)

		ticker := time.NewTicker(updateHeartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if send(protocol.Message{Type: "heartbeat"}) != nil {
					return
				}
			}
		}
	}()

	result := execute()

	cancel()
	<-done

	return result
}

func installSparkUpdate(ctx context.Context, version string, wireVersion int, idle func() error, client *sparkrelease.Client) (string, error) {
	if runtime.GOOS != "linux" || !sparkrelease.UpdateAvailable(buildinfo.SparkVersion(), version) {
		return "", errors.New("select a newer Spark release or a different development build from the panel")
	}

	executable, err := os.Executable()
	if err != nil {
		return "", err
	}

	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}

	if err := idle(); err != nil {
		return "", err
	}

	staged, err := client.Stage(ctx, version, runtime.GOARCH, filepath.Dir(executable))
	if err != nil {
		return "", err
	}

	defer func() { _ = os.Remove(staged) }()

	if err := verifySparkUpdate(ctx, staged, version, wireVersion); err != nil {
		return "", err
	}

	if err := idle(); err != nil {
		return "", err
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	previous := executable + ".previous"
	if err := os.Remove(previous); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if err := os.Link(executable, previous); err != nil {
		return "", fmt.Errorf("retain previous Spark executable: %w", err)
	}

	if err := os.Rename(staged, executable); err != nil {
		return "", fmt.Errorf("replace Spark executable: %w", err)
	}

	return executable, nil
}

func verifySparkUpdate(ctx context.Context, executable, version string, wireVersion int) error {
	ctx, cancel := context.WithTimeout(ctx, updateVersionCheckTimeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, executable, "--update-info").Output()
	if err != nil {
		return errors.New("downloaded Spark does not support verified remote updates")
	}

	var info protocol.SparkUpdateInfo
	if err := json.Unmarshal(output, &info); err != nil || info.Version != version || info.MinimumProtocol < 1 || info.MinimumProtocol > wireVersion || info.Protocol < wireVersion {
		return errors.New("downloaded Spark version is not compatible with this Campfire panel; update the panel first")
	}

	return nil
}
