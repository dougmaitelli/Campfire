package runtime

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

// TestLiveDockerLifecycle is opt-in because it pulls an image and creates real containers.
func TestLiveDockerLifecycle(t *testing.T) {
	if os.Getenv("CAMPFIRE_DOCKER_TEST") != "1" {
		t.Skip("set CAMPFIRE_DOCKER_TEST=1 to run against Docker")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	docker := Docker{}
	if _, err := docker.Run(ctx, nil, "info", "--format", "{{.ServerVersion}}"); err != nil {
		t.Fatalf("Docker preflight failed; no container created: %v", err)
	}

	d := runtimeFixture()
	d.ServerID = "live-" + rand.Text()
	d.Configuration.Ports = []model.ServerPort{{BindIP: "127.0.0.1", HostPort: 0, ContainerPort: 25565, Protocol: "tcp"}}
	d.Ember.Installer = &model.Installer{Image: "alpine:3.22", Entrypoint: "sh", Script: "echo installed > installed.txt"}
	d.Ember.Startup = "echo Ready; while read line; do echo \"$line\"; done"
	d.Ember.Stop = "^SIGTERM"

	m := New(ctx, t.TempDir())
	defer m.Close()

	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()

		_, _ = docker.Run(cleanup, nil, "rm", "--force", container(d.ServerID))
		_, _ = docker.Run(cleanup, nil, "rm", "--force", container(d.ServerID)+"-install")
	})

	backupID := "backup-" + rand.Text()
	for index, action := range []string{"install", "start", "stop", "backup", "restore", "start", "restart", "kill", "delete"} {
		operationID := action + rand.Text()
		if action == "backup" {
			operationID = backupID
		}

		status, err := m.Execute(ctx, d, model.RuntimeRequest{ServerID: d.ServerID, Action: action, OperationID: operationID, Content: backupID + ".tar.gz"})
		if err != nil {
			t.Fatal(err)
		}

		for status.Busy {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}

			status, err = m.Execute(ctx, d, model.RuntimeRequest{ServerID: d.ServerID, Action: "status"})
			if err != nil {
				t.Fatal(err)
			}
		}

		if status.Error != "" {
			t.Fatalf("%s failed: %s", action, status.Error)
		}

		if index == 1 {
			if _, err := m.Execute(ctx, d, model.RuntimeRequest{ServerID: d.ServerID, Action: "command", Content: "hello"}); err != nil {
				t.Fatal(err)
			}
		}

		if action == "backup" {
			file, err := m.Files(ctx, d, model.FileRequest{ServerID: d.ServerID, Operation: "read", Path: "installed.txt"})
			if err != nil {
				t.Fatal(err)
			}

			if _, err := m.Files(ctx, d, model.FileRequest{ServerID: d.ServerID, Operation: "write", Path: "installed.txt", Data: []byte("changed"), Revision: file.Revision}); err != nil {
				t.Fatal(err)
			}
		}

		if action == "restore" {
			file, err := m.Files(ctx, d, model.FileRequest{ServerID: d.ServerID, Operation: "read", Path: "installed.txt"})
			if err != nil || string(file.Data) != "installed\n" {
				t.Fatal("restoration did not recover original installed data", err)
			}
		}

		t.Logf("%s: %s", action, status.State)
	}
}
