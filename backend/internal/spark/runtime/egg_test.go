package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"campfire/internal/shared/model"
)

func TestEmberRuntimeEnvironmentAndTypedConfiguration(t *testing.T) {
	d := runtimeFixture()
	d.Configuration.MemoryMB = 1024
	d.Configuration.Variables = map[string]string{"LABEL": "quoted \"value\"\nwith newline", "SERVER_MEMORY": "forged"}
	d.Ember.Configs = []model.ConfigFile{{Path: "settings.json", Parser: "json", Values: map[string]string{"label": "{{LABEL}}", "literal": "001", "count": "7", "enabled": "true", "memory": "{{SERVER_MEMORY}}"}, Strings: []string{"label", "literal", "memory"}}}

	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "settings.json"), []byte(`{"unrelated":"retained"}`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := configure(directory, d); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(directory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}

	if object["label"] != d.Configuration.Variables["LABEL"] || object["literal"] != "001" || object["memory"] != "1024" || object["enabled"] != true || object["count"] != float64(7) || object["unrelated"] != "retained" {
		t.Fatal("typed JSON replacement changed meaning", string(data))
	}

	env := environment(d)
	if env["SERVER_MEMORY"] != "1024" || env["SERVER_PORT"] != "25565" || env["SERVER_IP"] != "0.0.0.0" {
		t.Fatal("built-in environment is not authoritative", env)
	}

	d.Ember.Startup = "sleep {{SERVER_MEMORY}}"

	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
}
