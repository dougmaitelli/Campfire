package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/agents"
	"campfire/internal/shared/agentidentity"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func deploymentFixture(id, server string) model.Deployment {
	return model.Deployment{
		ServerID:      server,
		Configuration: model.ServerConfiguration{Name: "Game", SparkID: id, EmberID: "ember", Image: "alpine:3.22", CPULimit: 1, MemoryMB: 128, DiskMB: 1024, Variables: map[string]string{"PASSWORD": "private-value"}, Ports: []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25565, ContainerPort: 25565, Protocol: "tcp"}}},
		Ember:         model.Ember{ID: "ember", Schema: 1, Name: "Game", Images: map[string]string{"Alpine": "alpine:3.22"}, Startup: "sleep 3600", WorkDir: "/home/container", User: "1000:1000", Variables: []model.Variable{{Name: "PASSWORD", Required: true}}, Installer: &model.Installer{Image: "alpine:3.22", Entrypoint: "sh", Script: "echo install"}},
	}
}

func TestUnnegotiatedCommandsDoNotMutateSpark(t *testing.T) {
	directory := t.TempDir()
	store := &ConfigurationStore{Directory: directory}
	deployment := deploymentFixture("node", "game")
	command := protocol.Command{ID: "capability-check", Name: protocol.ConfigureServer, Deadline: time.Now().Add(time.Second), Deployment: &deployment}

	denied := store.executeNegotiated(context.Background(), "node", command, nil)
	if denied.Error == nil || denied.Error.Code != "unsupported_capability" {
		t.Fatal("unnegotiated command accepted", denied.Error)
	}

	if _, err := os.Stat(filepath.Join(directory, "node")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("denied command changed storage", err)
	}

	accepted := store.executeNegotiated(context.Background(), "node", command, []string{protocol.CapabilityConfigure})
	if accepted.Error != nil || accepted.Receipt == nil {
		t.Fatal("supported configuration rejected", accepted.Error)
	}

	command.Name, command.Deployment, command.Runtime = protocol.ServerAction, nil, &model.RuntimeRequest{ServerID: "game", Action: "reinstall"}

	denied = store.executeNegotiated(context.Background(), "node", command, []string{protocol.CapabilityLifecycle, protocol.CapabilityDiskQuota})
	if denied.Error == nil || denied.Error.Code != "unsupported_capability" {
		t.Fatal("installer reran without upgrade support", denied.Error)
	}
}

func testWireCommands(t *testing.T, hub *agents.Hub, id, directory string) {
	t.Helper()

	var workers sync.WaitGroup
	for i := range 12 {
		workers.Add(1)
		go func() {
			defer workers.Done()

			deployment := deploymentFixture(id, fmt.Sprintf("server-%d", i))

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			receipt, err := hub.Deploy(ctx, id, deployment)
			if err != nil {
				t.Errorf("wire deploy: %v", err)
				return
			}

			path := filepath.Join(directory, "servers", id, deployment.ServerID+".json")

			data, err := os.ReadFile(path)
			if err != nil {
				t.Error(err)
				return
			}

			if receipt.ServerID != deployment.ServerID || receipt.Revision != agentidentity.Fingerprint(data) {
				t.Error("response matched the wrong configuration")
			}

			var saved model.Deployment
			if err := json.Unmarshal(data, &saved); err != nil || !reflect.DeepEqual(saved, deployment) {
				t.Errorf("configuration snapshot mismatch: %v", err)
			}

			if err := agentidentity.CheckPermissions(path); err != nil {
				t.Error(err)
			}
		}()
	}

	workers.Wait()

	_, err := hub.Execute(context.Background(), id, protocol.Command{Name: "unknown"})

	var failure *protocol.CommandError
	if !errors.As(err, &failure) || failure.Code != "unsupported_command" {
		t.Fatalf("unsupported command response: %v", err)
	}

	wrong := deploymentFixture("other-node", "wrong-placement")
	if _, err := hub.Deploy(context.Background(), id, wrong); !errors.As(err, &failure) || failure.Code != "configuration_rejected" {
		t.Fatalf("placement validation: %v", err)
	}
}

func TestConfigurationIdempotencyAndValidation(t *testing.T) {
	store := &ConfigurationStore{Directory: t.TempDir()}
	deployment := deploymentFixture("node", "server")

	first, err := store.Deploy(context.Background(), "node", deployment)
	if err != nil {
		t.Fatal(err)
	}

	repeat, err := store.Deploy(context.Background(), "node", deployment)
	if err != nil || first != repeat {
		t.Fatalf("non-idempotent storage: %v", err)
	}

	deployment.Configuration.Variables["PASSWORD"] = "changed"

	updated, err := store.Deploy(context.Background(), "node", deployment)
	if err != nil || updated.Revision == first.Revision {
		t.Fatalf("configuration revision did not change: %v", err)
	}

	deployment.ServerID = "../escape"
	if _, err := store.Deploy(context.Background(), "node", deployment); err == nil {
		t.Fatal("path traversal accepted")
	}

	result := store.execute(context.Background(), "node", protocol.Command{ID: "expired", Name: protocol.ConfigureServer, Deadline: time.Now().Add(-time.Second), Deployment: &deployment})
	if result.Error == nil || result.Error.Code != "invalid_deadline" {
		t.Fatal("expired command accepted")
	}
}

func TestEggRulesEnforcedOnSpark(t *testing.T) {
	store := &ConfigurationStore{Directory: t.TempDir()}
	d := deploymentFixture("node", "egg-rules")
	d.Ember.Variables = []model.Variable{{Name: "PLAYERS", Default: "20", Required: true, Editable: true, Validation: &model.VariableValidation{Type: "integer", Min: new(float64(1)), Max: new(float64(128))}}}

	d.Configuration.Variables = map[string]string{"PLAYERS": "129"}
	if _, err := store.Deploy(context.Background(), "node", d); err == nil {
		t.Fatal("Spark ignored imported maximum")
	}

	d.Configuration.Variables["PLAYERS"] = "20"
	if _, err := store.Deploy(context.Background(), "node", d); err != nil {
		t.Fatal("valid imported constraints rejected", err)
	}
}
