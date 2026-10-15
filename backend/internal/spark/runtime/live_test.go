package runtime

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
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

	quotaRoot := os.Getenv("CAMPFIRE_QUOTA_TEST_DIR")
	if quotaRoot == "" {
		t.Fatal("set CAMPFIRE_QUOTA_TEST_DIR to a project-quota-enabled filesystem directory")
	}

	directory, err := os.MkdirTemp(quotaRoot, "campfire-live-")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(directory) })

	m := New(ctx, directory)
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

// TestLiveDockerDiskQuota proves that the non-root game process receives EDQUOT
// without a polling loop, and that the quota survives a Spark manager restart.
func TestLiveDockerDiskQuota(t *testing.T) {
	if os.Getenv("CAMPFIRE_DOCKER_TEST") != "1" {
		t.Skip("set CAMPFIRE_DOCKER_TEST=1 and CAMPFIRE_QUOTA_TEST_DIR to run against Docker")
	}

	quotaRoot := os.Getenv("CAMPFIRE_QUOTA_TEST_DIR")
	if quotaRoot == "" {
		t.Fatal("CAMPFIRE_QUOTA_TEST_DIR must be on a project-quota-enabled filesystem")
	}

	directory, err := os.MkdirTemp(quotaRoot, "campfire-quota-")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(directory) })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	m := New(ctx, directory)
	t.Cleanup(m.Close)

	d := runtimeFixture()
	d.ServerID = "quota-" + rand.Text()
	d.Configuration.DiskMB = 8
	d.Configuration.Ports[0].HostPort = 0
	d.Ember.Ready = ""
	d.Ember.Stop = "^SIGTERM"
	d.Ember.Installer.Script = "echo installed > installed.txt"
	d.Ember.Startup = "mkdir -p nested; dd if=/dev/zero of=nested/full bs=1048576 count=32; echo QuotaAttemptComplete; while read line; do echo \"$line\"; done"

	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()

		_, _ = m.Docker.Run(cleanup, nil, "rm", "--force", container(d.ServerID))
		_, _ = m.Docker.Run(cleanup, nil, "rm", "--force", container(d.ServerID)+"-install")
	})

	for _, action := range []string{"install", "start"} {
		status, err := m.Execute(ctx, d, model.RuntimeRequest{ServerID: d.ServerID, Action: action, OperationID: action + rand.Text()})
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

		if status.Error != "" || !status.DiskQuotaEnforced {
			t.Fatalf("%s: %+v", action, status)
		}
	}

	for {
		logs, err := m.Docker.Run(ctx, nil, "logs", container(d.ServerID))
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(logs, "QuotaAttemptComplete") {
			if !strings.Contains(logs, "Disk quota exceeded") {
				t.Fatalf("game write did not hit EDQUOT: %s", logs)
			}

			break
		}

		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}

	info, err := os.Stat(filepath.Join(directory, d.ServerID, "data", "nested", "full"))
	if err != nil || info.Size() > 8<<20 {
		t.Fatalf("game exceeded allocation: %v", err)
	}

	m.Close()

	restarted := New(ctx, directory)
	t.Cleanup(restarted.Close)

	volume, err := restarted.volume(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	if err := restarted.Quota.Verify(volume, d.Configuration.DiskMB); err != nil {
		t.Fatal(err)
	}
}
