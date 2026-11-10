package model

import (
	"fmt"
	"maps"
	"testing"
)

func emberReviewFixture(t *testing.T) (Ember, Server) {
	t.Helper()

	ember := Ember{ID: "ember", Schema: 1, Name: "Game", Images: map[string]string{"Game": "game:one"}, Startup: "sleep {{DELAY}}", WorkDir: "/data", User: "1000", Variables: []Variable{{Name: "DELAY", Default: "60", Editable: true}}}
	server := Server{ID: "server", ServerConfiguration: ServerConfiguration{EmberID: ember.ID, SparkID: "spark", Name: "Server", Image: "game:one", Variables: map[string]string{"DELAY": "custom"}, CPULimit: 1, MemoryMB: 128, DiskMB: 1024}}

	deployment, err := NewDeployment(server.ID, server.ServerConfiguration, ember)
	if err != nil {
		t.Fatal(err)
	}

	server.EmberHash = DeploymentHash(deployment)

	return ember, server
}

func TestServerEmberMetadataDoesNotRequireReviewOrDeploymentSync(t *testing.T) {
	ember, server := emberReviewFixture(t)
	previous := EmberHash(ember)
	ember.Name, ember.Description, ember.ArtURL = "Renamed game", "Changed description", "/game-art/new.svg"
	ember.SavedataPaths = []SavedataPath{{Path: "new-save-location"}}
	server.SetEmberStatus(ember)

	if previous == EmberHash(ember) || server.EmberReviewRequired || server.DeploymentSyncRequired || server.EmberUpdateAvailable || !server.SavedataAvailable {
		t.Fatal("metadata was ignored by catalog detection or affected deployment", server)
	}
}

func TestServerEmberReviewIncludesEveryCompatibleValueChange(t *testing.T) {
	for _, change := range []string{"new empty variable", "new hidden variable", "locked default", "removed variable", "image", "startup choice"} {
		t.Run(change, func(t *testing.T) {
			ember, server := emberReviewFixture(t)

			switch change {
			case "new empty variable":
				ember.Variables = append(ember.Variables, Variable{Name: "NEW_VALUE", Editable: true})
			case "new hidden variable":
				ember.Variables = append(ember.Variables, Variable{Name: "HIDDEN_VALUE", Default: "private", Hidden: true})
			case "locked default":
				ember.Variables[0].Editable = false
			case "removed variable":
				server.Variables = maps.Clone(server.Variables)
				server.Variables["OBSOLETE"] = "old"
			case "image":
				ember.Images = map[string]string{"Game": "game:two"}
			case "startup choice":
				ember.Startup, ember.StartupCommands = "", map[string]string{"Standard": "sleep {{DELAY}}"}
			}

			server.SetEmberStatus(ember)

			if !server.EmberReviewRequired || !server.EmberUpdateAvailable || server.DeploymentSyncRequired {
				t.Fatal("a saved-value change bypassed review", server)
			}

			proposal := ember.ProposedServerConfiguration(server.ServerConfiguration)
			if proposal.Variables["DELAY"] != "custom" && change != "locked default" {
				t.Fatal("editable custom value was reset", proposal)
			}
		})
	}
}

func TestServerEmberRuntimeChangesNeedSyncWhileOverridesRemainAuthoritative(t *testing.T) {
	ember, server := emberReviewFixture(t)
	ember.Startup = "sleep {{DELAY}} # updated"
	server.SetEmberStatus(ember)

	if server.EmberReviewRequired || !server.DeploymentSyncRequired {
		t.Fatal("runtime-only change opened a value review or skipped sync", server)
	}

	server.StartupOverride = "sleep 42"

	deployment, err := NewDeployment(server.ID, server.ServerConfiguration, ember)
	if err != nil {
		t.Fatal(err)
	}

	server.EmberHash = DeploymentHash(deployment)
	ember.Startup = "sleep {{DELAY}} # another default"
	server.SetEmberStatus(ember)

	if server.EmberReviewRequired || server.DeploymentSyncRequired {
		t.Fatal("an overridden Ember default changed the effective deployment", server)
	}
}

func TestServerEmberInvalidExistingValueRequiresReview(t *testing.T) {
	ember, server := emberReviewFixture(t)
	ember.Variables[0].Validation = &VariableValidation{Type: VariableInteger}
	server.SetEmberStatus(ember)

	if !server.EmberReviewRequired {
		t.Fatal("invalid existing value did not require review")
	}
}

func TestServerEmberStatusRetainsSavedValueValidation(t *testing.T) {
	for _, change := range []string{"resources", "port", "environment", "startup override", "required port"} {
		t.Run(change, func(t *testing.T) {
			ember, server := emberReviewFixture(t)

			switch change {
			case "resources":
				server.MemoryMB = 0
			case "port":
				server.Ports = []ServerPort{{BindIP: "invalid", HostPort: 25565, ContainerPort: 25565, Protocol: NetworkTCP}}
			case "environment":
				server.Variables["DELAY"] = "unsafe\x00value"
			case "startup override":
				server.StartupOverride = "sleep {{MISSING}}"
			case "required port":
				ember.RequiredPorts = []RequiredPort{{Container: 25565, Protocol: NetworkUDP}}
				server.Ports = []ServerPort{{BindIP: "0.0.0.0", HostPort: 25565, ContainerPort: 25565, Protocol: NetworkTCP}}
			}

			server.SetEmberStatus(ember)

			if !server.EmberReviewRequired || server.DeploymentSyncRequired {
				t.Fatal("invalid saved values escaped review", server)
			}
		})
	}
}

func BenchmarkServerEmberStatus(b *testing.B) {
	ember := Ember{ID: "ember", Schema: 1, Name: "Game", Images: map[string]string{"Game": "game:one"}, Startup: "sleep 60", WorkDir: "/data", User: "1000"}
	server := Server{ID: "server", ServerConfiguration: ServerConfiguration{EmberID: ember.ID, SparkID: "spark", Name: "Server", Image: "game:one", Variables: map[string]string{}, CPULimit: 1, MemoryMB: 128, DiskMB: 1024}}

	for i := range 128 {
		name := fmt.Sprintf("SETTING_%d", i)
		ember.Variables = append(ember.Variables, Variable{Name: name, Default: "1", Editable: true, Pattern: "^[0-9]+$"})
		server.Variables[name] = "2"
	}

	deployment, err := NewDeployment(server.ID, server.ServerConfiguration, ember)
	if err != nil {
		b.Fatal(err)
	}

	server.EmberHash = DeploymentHash(deployment)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		server.SetEmberStatus(ember)
	}
}

func TestServerViewDoesNotExposeTheHiddenValueDeploymentFingerprint(t *testing.T) {
	ember, server := emberReviewFixture(t)
	ember.Variables = append(ember.Variables, Variable{Name: "PRIVATE_FLAG", Hidden: true, Editable: true})
	server.Variables["PRIVATE_FLAG"] = "one"

	deployment, err := NewDeployment(server.ID, server.ServerConfiguration, ember)
	if err != nil {
		t.Fatal(err)
	}

	server.EmberHash = DeploymentHash(deployment)

	view := VisibleServer(server, ember)
	if view.EmberHash != EmberHash(ember) || view.EmberHash == server.EmberHash {
		t.Fatal("public view exposed an execution fingerprint")
	}

	if _, exists := view.Variables["PRIVATE_FLAG"]; exists {
		t.Fatal("public view exposed hidden values")
	}
}
