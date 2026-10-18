package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"campfire/internal/shared/model"
)

func TestCleanReinstallDeletesGameFilesAndKeepsBackups(t *testing.T) {
	d := runtimeFixture()
	m := newTestManager(context.Background(), t.TempDir())
	m.Docker = &fakeDocker{states: make(map[string]string)}
	m.Ownership = func(string, string) error { return nil }
	t.Cleanup(m.Close)

	if status := awaitOperation(t, m, d, "install", "initial"); status.Error != "" {
		t.Fatal(status.Error)
	}

	volume := filepath.Join(m.Directory, d.ServerID, "data")

	backups := filepath.Join(m.Directory, d.ServerID, "backups")
	if err := os.MkdirAll(backups, 0700); err != nil {
		t.Fatal(err)
	}

	backup := filepath.Join(backups, "retained.tar.gz")

	world := filepath.Join(volume, "world.bin")
	for _, name := range []string{world, backup} {
		if err := os.WriteFile(name, []byte("retained"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Deleting a symlink in the game directory must not follow it into backups.
	if err := os.Symlink(backups, filepath.Join(volume, "linked-backups")); err != nil {
		t.Fatal(err)
	}

	request := model.RuntimeRequest{ServerID: d.ServerID, Action: "reinstall", Content: "clean", OperationID: "clean"}
	if status := awaitOperation(t, m, d, "start", "start"); status.Error != "" {
		t.Fatal(status.Error)
	}

	if _, err := m.Execute(context.Background(), d, request); err == nil {
		t.Fatal("accepted clean install against running server")
	}

	if _, err := os.Stat(world); err != nil {
		t.Fatal("rejected request removed game files", err)
	}

	if status := awaitOperation(t, m, d, "stop", "stop"); status.Error != "" {
		t.Fatal(status.Error)
	}

	if status := awaitRequest(t, m, d, request); status.Error != "" || status.State != "stopped" {
		t.Fatal(status)
	}

	if _, err := os.Stat(world); !os.IsNotExist(err) {
		t.Fatal("clean install retained world", err)
	}

	if data, err := os.ReadFile(backup); err != nil || string(data) != "retained" {
		t.Fatal("clean install changed backup", err)
	}

	request.Content = "invalid"

	request.OperationID = "invalid"
	if _, err := m.Execute(context.Background(), d, request); err == nil {
		t.Fatal("accepted invalid reinstall mode")
	}
}

func TestUpdateAndReinstallPreserveFilesAndFenceFailedInstall(t *testing.T) {
	d := runtimeFixture()
	docker := &fakeDocker{states: make(map[string]string)}
	directory := t.TempDir()
	m := newTestManager(context.Background(), directory)
	m.Docker = docker
	m.Ownership = func(string, string) error { return nil }
	t.Cleanup(m.Close)

	if status := awaitOperation(t, m, d, "install", "initial"); status.Error != "" {
		t.Fatal(status.Error)
	}

	world := filepath.Join(directory, d.ServerID, "data", "world.bin")

	backups := filepath.Join(directory, d.ServerID, "backups")
	if err := os.MkdirAll(backups, 0700); err != nil {
		t.Fatal(err)
	}

	backup := filepath.Join(backups, "retained.tar.gz")
	for _, name := range []string{world, backup} {
		if err := os.WriteFile(name, []byte("retained game data"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	if status := awaitOperation(t, m, d, "start", "running"); status.Error != "" {
		t.Fatal(status.Error)
	}

	for _, action := range []string{"update", "reinstall"} {
		if _, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: action, OperationID: action + "-running"}); err == nil {
			t.Fatal("accepted installer against running game")
		}
	}

	if status := awaitOperation(t, m, d, "stop", "stop"); status.Error != "" {
		t.Fatal(status.Error)
	}

	for _, action := range []string{"update", "reinstall"} {
		docker.mu.Lock()
		docker.calls = nil
		docker.mu.Unlock()

		if status := awaitOperation(t, m, d, action, action); status.Error != "" || status.State != "stopped" {
			t.Fatal(status)
		}

		docker.mu.Lock()

		var pulls, installers int

		for _, call := range docker.calls {
			if call[0] == "pull" {
				pulls++
			}

			if call[0] == "run" {
				installers++
			}
		}
		docker.mu.Unlock()

		if pulls == 0 || installers != 1 {
			t.Fatalf("%s: pulls=%d installers=%d", action, pulls, installers)
		}

		for _, name := range []string{world, backup} {
			data, err := os.ReadFile(name)
			if err != nil || string(data) != "retained game data" {
				t.Fatal("existing data changed", name, err)
			}
		}
	}

	docker.mu.Lock()
	docker.failInstaller = true
	docker.mu.Unlock()

	if status := awaitOperation(t, m, d, "reinstall", "failed"); status.Error == "" {
		t.Fatal("installer failure hidden")
	}

	m.Close()
	m = newTestManager(context.Background(), directory)
	m.Docker = docker
	m.Ownership = func(string, string) error { return nil }
	t.Cleanup(m.Close)

	if _, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: "start", OperationID: "unsafe-start"}); err == nil {
		t.Fatal("failed reinstall could be started after restart")
	}

	if data, err := os.ReadFile(world); err != nil || string(data) != "retained game data" {
		t.Fatal("failure lost world", err)
	}

	docker.mu.Lock()
	docker.failInstaller = false
	docker.mu.Unlock()

	if status := awaitOperation(t, m, d, "reinstall", "retry"); status.Error != "" {
		t.Fatal(status.Error)
	}

	if status := awaitOperation(t, m, d, "start", "start-after-retry"); status.Error != "" {
		t.Fatal(status.Error)
	}
}
