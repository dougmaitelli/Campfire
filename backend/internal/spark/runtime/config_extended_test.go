package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestStructuredConfigNestedPathsWildcardsAndPrecision(t *testing.T) {
	old := []byte(`{"listeners":[{"port":1},{"port":2}],"big":9007199254740993,"unchanged":true}`)
	config := model.ConfigFile{Parser: "json", Values: map[string]string{"listeners.*.port": "25565", "listeners.0.label": "001", "network.enabled": "true"}, Strings: []string{"listeners.0.label"}}

	updated, err := structuredConfig(old, config, config.Values)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(updated), "9007199254740993") || strings.Count(string(updated), "25565") != 2 || !strings.Contains(string(updated), `"001"`) {
		t.Fatal("nested rewrite changed data", string(updated))
	}

	var object map[string]any
	if err := json.Unmarshal(updated, &object); err != nil {
		t.Fatal(err)
	}

	network, ok := object["network"].(map[string]any)
	if !ok {
		t.Fatal("network mapping missing")
	}

	if network["enabled"] != true || object["unchanged"] != true {
		t.Fatal("types were changed", object)
	}

	for _, key := range []string{"listeners.4096.port", "listeners.-1.port", "missing.*.port", "big.port"} {
		bad := model.ConfigFile{Parser: "json", Values: map[string]string{key: "1"}}
		if _, err := structuredConfig(old, bad, bad.Values); err == nil {
			t.Fatal("accepted invalid structural rewrite", key)
		}
	}

	config = model.ConfigFile{Parser: "json", Values: map[string]string{"listeners.*.label": strings.Repeat("x", 600000)}, Strings: []string{"listeners.*.label"}}
	if _, err := structuredConfig(old, config, config.Values); err == nil {
		t.Fatal("accepted excessive output expansion")
	}
}

func TestConfigParsersCreateMissingFilesAndPreserveValues(t *testing.T) {
	d := runtimeFixture()
	directory := t.TempDir()
	d.Configuration.Variables = map[string]string{"LABEL": "001", "STOP_REASON": "maintenance"}
	d.Ember.Configs = []model.ConfigFile{
		{Path: "nested/settings.json", Parser: "json", Create: true, Values: map[string]string{"network.port": "{{SERVER_PORT}}"}, Strings: []string{"network.port"}},
		{Path: "settings.yaml", Parser: "yaml", Create: true, Values: map[string]string{"network.port": "25565", "network.label": "{{LABEL}}", "big": "9007199254740993"}, Strings: []string{"network.label"}},
		{Path: "settings.ini", Parser: "ini", Create: true, Values: map[string]string{"[game.network].port": "{{SERVER_PORT}}"}},
		{Path: "settings.properties", Parser: "properties", Create: true, Values: map[string]string{"port": "{{SERVER_PORT}}"}},
		{Path: "startup.cfg", Parser: "file", Create: true, Values: map[string]string{"port ": "port {{SERVER_PORT}}"}},
	}

	if err := os.WriteFile(filepath.Join(directory, "startup.cfg"), []byte("# comment\nport 1\nunrelated yes\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := configure(directory, d); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct{ path, expected string }{{"nested/settings.json", `"25565"`}, {"settings.yaml", `"001"`}, {"settings.ini", "25565"}, {"settings.properties", "port=25565"}, {"startup.cfg", "port 25565\nunrelated yes"}} {
		data, err := os.ReadFile(filepath.Join(directory, test.path))
		if err != nil || !strings.Contains(string(data), test.expected) {
			t.Fatal("config mismatch", test.path, string(data), err)
		}
	}

	for _, source := range []string{"a: &ref {value: 1}\nb: *ref\n", "a: 1\na: 2\n", "a: !secret hello\n", "a: 1\n---\na: 2\n"} {
		config := model.ConfigFile{Parser: "yaml", Values: map[string]string{"a": "2"}}
		if _, err := structuredConfig([]byte(source), config, config.Values); err == nil {
			t.Fatal("accepted ambiguous YAML", source)
		}
	}

	if err := os.Symlink(t.TempDir(), filepath.Join(directory, "link")); err != nil {
		t.Fatal(err)
	}

	d.Ember.Configs = []model.ConfigFile{{Path: "link/outside.json", Parser: "json", Create: true, Values: map[string]string{"port": "1"}}}
	if err := configure(directory, d); err == nil {
		t.Fatal("config creation followed a symlink")
	}
}

func TestReadinessAlternativesRegexANSIAndStopTemplates(t *testing.T) {
	ember := model.Ember{Readiness: &model.Readiness{StripANSI: true, Any: []model.ReadinessMatch{{Value: "Ready for connections"}, {Value: "(?i)^Listening on port [0-9]+$", Regex: true}}}}
	if !readinessMatches(ember, "noise\n\x1b[32mLISTENING on port 25565\x1b[0m\n") || !readinessMatches(ember, "Ready for connections") || readinessMatches(ember, "not ready") {
		t.Fatal("readiness matching failed")
	}

	d := runtimeFixture()
	d.Ember.Stop = "quit {{REASON}}"

	d.Configuration.Variables = map[string]string{"REASON": "maintenance"}
	if command, err := stopCommand(d); err != nil || command != "quit maintenance" {
		t.Fatal("stop template mismatch", command, err)
	}

	d.Configuration.Variables["REASON"] = "maintenance\nmalicious command"
	if _, err := stopCommand(d); err == nil {
		t.Fatal("stop template allowed command injection")
	}
}
