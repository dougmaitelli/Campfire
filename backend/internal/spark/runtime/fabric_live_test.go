package runtime

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func liveOperation(ctx context.Context, t *testing.T, m *Manager, d model.Deployment, action string) model.RuntimeStatus {
	t.Helper()

	status, err := m.Execute(ctx, d, model.RuntimeRequest{ServerID: d.ServerID, Action: action, OperationID: rand.Text()})
	if err != nil {
		t.Fatal(err)
	}

	for status.Busy {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}

		status, err = m.Execute(ctx, d, model.RuntimeRequest{ServerID: d.ServerID, Action: "status"})
		if err != nil {
			t.Fatal(err)
		}
	}

	return status
}

func cleanupLiveContainers(t *testing.T, m *Manager, id string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		_, _ = m.Docker.Run(ctx, nil, "rm", "--force", container(id)+"-install", container(id))
		m.Close()
	})
}

// TestLiveFabricInstallation exercises the upstream Egg, including package installation and downloads.
func TestLiveFabricInstallation(t *testing.T) {
	if os.Getenv("CAMPFIRE_FABRIC_TEST") != "1" {
		t.Skip("set CAMPFIRE_FABRIC_TEST=1 to download and install Fabric with Docker")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Minute)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/pterodactyl/game-eggs/main/minecraft/java/fabric/egg-fabric.json", nil)
	if err != nil {
		t.Fatal(err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		t.Fatal("upstream Egg download", response.Status)
	}

	var egg struct {
		Scripts struct {
			Installation struct{ Script, Container, Entrypoint string }
		}
		Variables []struct {
			Name    string `json:"env_variable"`
			Default string `json:"default_value"`
		}
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&egg); err != nil {
		t.Fatal(err)
	}

	m := New(ctx, t.TempDir())
	d := runtimeFixture()
	d.ServerID = "fabric-live-" + rand.Text()
	d.Configuration.MemoryMB, d.Configuration.CPULimit, d.Configuration.DiskMB = 4096, 2, 0

	d.Configuration.Variables = make(map[string]string)
	for _, variable := range egg.Variables {
		d.Configuration.Variables[variable.Name] = variable.Default
	}

	installer := egg.Scripts.Installation
	d.Ember.Installer = &model.Installer{Image: installer.Container, Entrypoint: installer.Entrypoint, Script: installer.Script}
	cleanupLiveContainers(t, m, d.ServerID)
	status := liveOperation(ctx, t, m, d, "install")

	var output strings.Builder

	if err := m.StreamConsole(ctx, d, func(text string) error { output.WriteString(text); return nil }); err != nil {
		t.Fatal(err)
	}

	t.Log("Installer output:\n" + output.String())

	if status.OperationState != model.OperationSucceeded {
		t.Fatalf("Fabric installer failed: %+v", status)
	}

	for _, name := range []string{"server.jar", "minecraft-server.jar", "fabric-server-launcher.properties"} {
		info, err := os.Stat(filepath.Join(m.root(d.ServerID), "data", name))
		if err != nil || info.Size() == 0 {
			t.Fatalf("missing or empty %s: %v", name, err)
		}
	}

	if !strings.Contains(output.String(), "Install Complete") {
		t.Fatal("console did not show the completed install")
	}
}

// TestLiveCrashLoopRecovery verifies manual actions against Docker's actual restarting state.
func TestLiveCrashLoopRecovery(t *testing.T) {
	if os.Getenv("CAMPFIRE_DOCKER_TEST") != "1" {
		t.Skip("set CAMPFIRE_DOCKER_TEST=1 to test crash-loop recovery with Docker")
	}

	for _, action := range []string{"stop", "kill", "delete"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()

			m := New(ctx, t.TempDir())
			d := runtimeFixture()
			d.ServerID = "crash-live-" + rand.Text()
			d.Configuration.DiskMB = 0
			d.Configuration.Ports[0].HostPort = 0
			d.Ember.Installer = nil
			d.Ember.Startup = "exit 1"
			cleanupLiveContainers(t, m, d.ServerID)

			if status := liveOperation(ctx, t, m, d, "install"); status.ErrorCode != "" {
				t.Fatal(status)
			}

			if status := liveOperation(ctx, t, m, d, "start"); status.ErrorCode != "" {
				t.Fatal(status)
			}

			for {
				state, err := m.inspect(ctx, d.ServerID)
				if err != nil {
					t.Fatal(err)
				}

				if state == "restarting" {
					break
				}

				select {
				case <-ctx.Done():
					t.Fatal("container never entered restarting state")
				case <-time.After(50 * time.Millisecond):
				}
			}

			status := liveOperation(ctx, t, m, d, action)
			if status.ErrorCode != "" || status.State != "stopped" && status.State != "deleted" {
				t.Fatal("crash-loop recovery failed", status)
			}

			time.Sleep(500 * time.Millisecond)

			state, err := m.inspect(ctx, d.ServerID)
			if err != nil || state != "exited" && state != "missing" {
				t.Fatal("container resumed its crash loop", state, err)
			}
		})
	}
}
