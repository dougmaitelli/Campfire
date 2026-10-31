package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func deploymentSource() (ServerConfiguration, Ember) {
	c := ServerConfiguration{
		Name: "Panel server name", Description: "Panel server description", SparkID: "node", EmberID: "game",
		Image: "alpine:3.22", Variables: map[string]string{"PASSWORD": "literal $(value)"},
		CPULimit: 1, MemoryMB: 128, DiskMB: 1024, GameIntegration: true,
	}
	e := Ember{
		ID: "game", Schema: 1, Name: "Panel game name", Description: "Panel game description", Revision: "revision",
		ArtURL: "https://example.test/art.png", UpdateURL: "https://example.test/game.json",
		Images:  map[string]string{"Selected": "alpine:3.22", "Alternative": "alpine:3.21"},
		Startup: "echo {{PASSWORD}}", StartupShell: "bash", WorkDir: "/home/container", User: "1000:1000",
		Variables: []Variable{{Name: "PASSWORD", Default: "default", Required: true, Hidden: true, Validation: &VariableValidation{Type: "string"}}},
		Installer: &Installer{Image: "alpine:3.22", Entrypoint: "sh", Script: "echo install"},
		Stop:      "quit", Readiness: &Readiness{Any: []ReadinessMatch{{Value: "^Ready$", Regex: true}}},
		Configs:       []ConfigFile{{Path: "settings.json", Parser: "json", Create: true, Values: map[string]string{"password": "{{PASSWORD}}"}, Strings: []string{"password"}}},
		RequiredPorts: []RequiredPort{{Container: 25565, Protocol: "tcp", Description: "Panel port description"}},
		Origin:        &EmberOrigin{Format: "pelican", Author: "Panel author", Warnings: []string{"Panel warning"}},
	}

	return c, e
}

func TestDeploymentContainsOnlyExecutionData(t *testing.T) {
	c, e := deploymentSource()

	d, err := NewDeployment("server", c, e)
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}

	var wire struct {
		Configuration map[string]json.RawMessage `json:"configuration"`
		Ember         map[string]json.RawMessage `json:"ember"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"name", "description", "startup_command", "startup_override"} {
		if _, ok := wire.Configuration[key]; ok {
			t.Fatal("panel setting leaked into execution configuration", key)
		}
	}

	for _, key := range []string{"id", "schema", "name", "description", "revision", "artUrl", "updateUrl", "images", "startup_commands", "variables", "required_ports", "origin"} {
		if _, ok := wire.Ember[key]; ok {
			t.Fatal("authoring metadata leaked into execution definition", key)
		}
	}

	if d.Configuration.Variables["PASSWORD"] != c.Variables["PASSWORD"] || d.Configuration.Variables["SERVER_PORT"] != "25565" || !d.Configuration.GameIntegration || d.Ember.Startup != e.Startup || !reflect.DeepEqual(d.Ember.Configs, e.Configs) || !reflect.DeepEqual(d.Ember.Installer, e.Installer) || !reflect.DeepEqual(d.Ember.Readiness, e.Readiness) {
		t.Fatal("execution instructions changed during projection", string(data))
	}

	if _, changed := c.Variables["SERVER_PORT"]; changed {
		t.Fatal("projection mutated the saved panel variables")
	}

	decoded, err := DecodeStoredDeployment(data)
	if err != nil || !reflect.DeepEqual(decoded, d) {
		t.Fatal("execution snapshot did not round trip", err)
	}
}

func TestDeploymentResolvesStartupBeforeDelivery(t *testing.T) {
	for _, kind := range []string{"default", "named", "override"} {
		t.Run(kind, func(t *testing.T) {
			c, e := deploymentSource()

			want := e.Startup
			if kind != "default" {
				e.Startup = ""
				e.StartupCommands = map[string]string{"Normal": "echo normal", "Debug": "echo debug; echo {{PASSWORD}}"}
				c.StartupCommand, want = "Debug", e.StartupCommands["Debug"]
			}

			if kind == "override" {
				c.StartupOverride, want = "echo custom; echo {{PASSWORD}}", "echo custom; echo {{PASSWORD}}"
			}

			d, err := NewDeployment("server", c, e)
			if err != nil || d.Ember.Startup != want {
				t.Fatal("startup was not resolved on the panel", d.Ember.Startup, err)
			}

			// Old Spark snapshots must preserve the same selection after an upgrade.
			legacy, err := json.Marshal(struct {
				ServerID      string              `json:"server_id"`
				Configuration ServerConfiguration `json:"configuration"`
				Ember         Ember               `json:"ember"`
			}{"server", c, e})
			if err != nil {
				t.Fatal(err)
			}

			converted, err := DecodeStoredDeployment(legacy)
			if err != nil || !reflect.DeepEqual(converted, d) {
				t.Fatal("legacy snapshot changed execution instructions", err)
			}
		})
	}
}

func TestDeploymentRejectsInvalidPanelSelections(t *testing.T) {
	for _, kind := range []string{"image", "variable", "selection", "override", "identity"} {
		t.Run(kind, func(t *testing.T) {
			c, e := deploymentSource()

			switch kind {
			case "image":
				c.Image = "unselected:latest"
			case "variable":
				c.Variables["PASSWORD"] = ""
			case "selection":
				c.StartupCommand = "removed"
			case "override":
				c.StartupOverride = "echo {{UNKNOWN}}"
			case "identity":
				c.EmberID = "other"
			}

			if _, err := NewDeployment("server", c, e); err == nil {
				t.Fatal("invalid panel selection was projected", kind)
			}
		})
	}
}

func TestStoredDeploymentRejectsInvalidExecution(t *testing.T) {
	c, e := deploymentSource()

	d, err := NewDeployment("server", c, e)
	if err != nil {
		t.Fatal(err)
	}

	d.Ember.User = "0:0"

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := DecodeStoredDeployment(data); err == nil || !strings.Contains(err.Error(), "non-root") {
		t.Fatal("unsafe stored execution instructions accepted", err)
	}
}
