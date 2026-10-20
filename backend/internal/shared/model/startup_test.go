package model

import "testing"

func TestNamedStartupSelectionAndValidation(t *testing.T) {
	ember := Ember{Schema: 1, Name: "Game", Images: map[string]string{"Game": "alpine:3.22"}, WorkDir: "/data", User: "1000", StartupCommands: map[string]string{"Normal": "echo {{SERVER_MEMORY}}", "Debug": "echo debug"}}
	if err := ember.Validate(); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"", "missing"} {
		if _, _, err := ember.ResolveStartup(name); err == nil {
			t.Fatal("missing selection was accepted", name)
		}
	}

	key, command, err := ember.ResolveStartup("Debug")
	if err != nil || key != "Debug" || command != "echo debug" {
		t.Fatal(key, command, err)
	}

	ember.StartupCommands["Normal"] = "echo {{UNKNOWN}}"
	if err := ember.Validate(); err == nil {
		t.Fatal("unselected invalid command was accepted")
	}

	delete(ember.StartupCommands, "Normal")

	key, _, err = ember.ResolveStartup("")
	if err != nil || key != "Debug" {
		t.Fatal("single command was not normalized to its key", key, err)
	}

	if _, _, err := ember.ResolveStartup("Normal"); err == nil {
		t.Fatal("removed selection silently switched to the remaining command")
	}

	ember.Startup = "echo legacy"
	if err := ember.Validate(); err == nil {
		t.Fatal("ambiguous single and named startup fields were accepted")
	}

	ember.StartupCommands = nil

	key, command, err = ember.ResolveStartup("")
	if err != nil || key != "" || command != "echo legacy" {
		t.Fatal("single startup changed", key, command, err)
	}
}
