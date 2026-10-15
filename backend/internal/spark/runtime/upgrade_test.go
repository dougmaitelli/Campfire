package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"campfire/internal/shared/model"
)

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

		if (action == "update" && pulls == 0) || (action == "reinstall" && pulls != 0) || installers != 1 {
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
