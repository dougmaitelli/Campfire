package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

type backupLifecycleDocker struct {
	*fakeDocker
	failStart   bool
	failStop    bool
	beforeStart func() error
	stopStarted chan struct{}
	allowStop   <-chan struct{}
}

func (d backupLifecycleDocker) Run(ctx context.Context, input io.Reader, args ...string) (string, error) {
	if args[0] == "stop" && d.stopStarted != nil {
		close(d.stopStarted)

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-d.allowStop:
		}
	}

	if args[0] == "stop" && d.failStop {
		return "", errors.New("stop failed")
	}

	if args[0] == "start" {
		if d.failStart {
			return "", errors.New("start failed")
		}

		if d.beforeStart != nil {
			if err := d.beforeStart(); err != nil {
				return "", err
			}
		}
	}

	return d.fakeDocker.Run(ctx, input, args...)
}

func TestStoppedBackupRemainsOneExclusiveOperation(t *testing.T) {
	m, d := fileTestManager(t)
	m.Ownership = func(string, string) error { return nil }
	awaitOperation(t, m, d, "install", "install")
	awaitOperation(t, m, d, "start", "start")

	started, allowed := make(chan struct{}), make(chan struct{})

	release := sync.OnceFunc(func() { close(allowed) })
	defer release()

	docker, ok := m.Docker.(*fakeDocker)
	if !ok {
		t.Fatal("expected the fixture Docker executor")
	}

	m.Docker = backupLifecycleDocker{fakeDocker: docker, stopStarted: started, allowStop: allowed}
	request := model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionBackup, OperationID: "scheduled-backup", BackupOptions: model.BackupOptions{StopServer: true}}

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	if _, err := m.Execute(ctx, d, request); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	status, err := m.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStatus})
	if err != nil || !status.Busy || status.State != model.RuntimeStateStopping || status.OperationID != request.OperationID {
		t.Fatal("stop phase lost the backup operation", status, err)
	}

	if _, err := m.Execute(t.Context(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerActionStart, OperationID: "other-start"}); err == nil {
		t.Fatal("another start was accepted during the backup sequence")
	}

	release()

	status = awaitBackup(t, m, d, request)
	if status.Error != "" || status.State != model.RuntimeStateRunning || status.OperationID != request.OperationID {
		t.Fatal("backup sequence did not finish under its original operation", status)
	}
}

func TestLiveBackupAndTemporaryStop(t *testing.T) {
	for _, scenario := range []struct {
		name                                                 string
		running, stopServer, failBackup, failStart, failStop bool
		wantState                                            model.RuntimeState
	}{
		{name: "live", running: true, wantState: model.RuntimeStateRunning},
		{name: "already stopped", wantState: model.RuntimeStateStopped},
		{name: "temporary stop", running: true, stopServer: true, wantState: model.RuntimeStateRunning},
		{name: "temporary stop preserves stopped", stopServer: true, wantState: model.RuntimeStateStopped},
		{name: "backup failure restarts", running: true, stopServer: true, failBackup: true, wantState: model.RuntimeStateRunning},
		{name: "restart failure is reported", running: true, stopServer: true, failStart: true, wantState: model.RuntimeStateStopped},
		{name: "stop failure aborts backup", running: true, stopServer: true, failStop: true, wantState: model.RuntimeStateRunning},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			m, d := fileTestManager(t)

			m.Ownership = func(string, string) error { return nil }
			if status := awaitOperation(t, m, d, "install", "install"); status.Error != "" {
				t.Fatal(status.Error)
			}

			if scenario.running {
				if status := awaitOperation(t, m, d, "start", "start"); status.Error != "" {
					t.Fatal(status.Error)
				}
			}

			volume := filepath.Join(m.root(d.ServerID), "data")
			if err := os.WriteFile(filepath.Join(volume, "world.dat"), []byte("saved world"), 0600); err != nil {
				t.Fatal(err)
			}

			if scenario.failBackup {
				if err := os.Symlink(t.TempDir(), filepath.Join(volume, "unsafe-link")); err != nil {
					t.Fatal(err)
				}
			}

			docker, ok := m.Docker.(*fakeDocker)
			if !ok {
				t.Fatal("expected the fixture Docker executor")
			}

			docker.mu.Lock()
			docker.calls = nil
			docker.mu.Unlock()

			archive := filepath.Join(m.root(d.ServerID), "backups", "full-snapshot.tar.gz")
			m.Docker = backupLifecycleDocker{fakeDocker: docker, failStart: scenario.failStart, failStop: scenario.failStop, beforeStart: func() error {
				if !scenario.failBackup {
					_, err := os.Stat(archive)
					return err
				}

				return nil
			}}
			status := awaitBackup(t, m, d, model.RuntimeRequest{Action: model.ServerActionBackup, OperationID: "snapshot", BackupOptions: model.BackupOptions{StopServer: scenario.stopServer}})

			wantError := scenario.failBackup || scenario.failStart || scenario.failStop
			if (status.Error != "") != wantError || status.State != scenario.wantState || status.Operation != model.ServerActionBackup || status.OperationID != "snapshot" {
				t.Fatal("backup did not preserve lifecycle state", status)
			}

			_, archiveErr := os.Stat(archive)
			if (archiveErr == nil) != (!scenario.failBackup && !scenario.failStop) {
				t.Fatal("backup archive publication did not match the result", archiveErr)
			}

			if scenario.failStart && !strings.Contains(status.Error, "restart after backup") {
				t.Fatal("restart failure was not identified", status.Error)
			}

			if !scenario.stopServer || !scenario.running {
				docker.mu.Lock()
				defer docker.mu.Unlock()

				for _, call := range docker.calls {
					if call[0] == "stop" || call[0] == "start" || call[0] == "create" {
						t.Fatal("backup changed a container without needing to", call)
					}
				}
			}
		})
	}
}
