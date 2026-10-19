package runtime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestValheimBashStartupAndLiteralVariableValues(t *testing.T) {
	data, err := os.ReadFile("../../campfire/service/testdata/eggs/valheim.json")
	if err != nil {
		t.Fatal(err)
	}

	var egg struct {
		Startup   string `json:"startup"`
		Variables []struct {
			Name    string `json:"env_variable"`
			Default string `json:"default_value"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(data, &egg); err != nil {
		t.Fatal(err)
	}

	d := runtimeFixture()
	d.Ember.Startup, d.Ember.StartupShell = egg.Startup, "bash"

	d.Configuration.Variables = map[string]string{}
	for _, variable := range egg.Variables {
		d.Configuration.Variables[variable.Name] = variable.Default
	}
	// Shell-looking characters in user input must remain literal arguments.
	d.Configuration.Variables["SERVER_NAME"] = `My $(touch injected) Server`
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)

	docker := &fakeDocker{states: map[string]string{}}
	m.Docker = docker

	m.Ownership = func(string, string) error { return nil }
	if status := awaitOperation(t, m, d, "install", "install-bash"); status.Error != "" {
		t.Fatal(status)
	}

	if status := awaitOperation(t, m, d, "start", "start-bash"); status.Error != "" {
		t.Fatal(status)
	}

	var create []string

	docker.mu.Lock()
	for _, args := range docker.calls {
		if args[0] == "create" {
			create = args
		}
	}
	docker.mu.Unlock()

	if len(create) < 3 || create[len(create)-3] != "bash" || create[len(create)-2] != "-c" {
		t.Fatal("startup shell was not passed to Docker", create)
	}

	tmpfs := slices.Index(create, "--tmpfs")
	if tmpfs < 0 || tmpfs+1 >= len(create) || create[tmpfs+1] != "/tmp:rw,exec,nosuid,nodev,size=100m,mode=1777" {
		t.Fatal("Valheim NSS wrapper needs a fresh writable temporary directory", create)
	}

	startup := create[len(create)-1]

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if out, err := exec.CommandContext(ctx, "bash", "-n", "-c", startup).CombinedOutput(); err != nil {
		t.Fatalf("invalid Bash startup: %v: %s", err, out)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "valheim_server.x86_64"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\nprintf '\\n(Filename: test Line: 123)\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}

	command := exec.CommandContext(ctx, "bash", "-c", startup)

	command.Dir, command.Env = dir, os.Environ()
	for key, value := range environment(d) {
		command.Env = append(command.Env, key+"="+value)
	}

	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Bash startup failed: %v: %s", err, out)
	}

	if !strings.Contains(string(out), "-crossplay\n") || !strings.Contains(string(out), d.Configuration.Variables["SERVER_NAME"]+"\n") || strings.Contains(string(out), "Filename:") {
		t.Fatalf("startup changed arguments or failed to filter output: %s", out)
	}

	if _, err := os.Stat(filepath.Join(dir, "injected")); !os.IsNotExist(err) {
		t.Fatal("variable contents were executed as shell commands", err)
	}
}

func TestNativeStartupShellValidation(t *testing.T) {
	ember := runtimeFixture().Ember

	ember.Startup = "sleep 60"
	for _, shell := range []string{"", "sh", "bash", "ash"} {
		ember.StartupShell = shell
		if err := ember.Validate(); err != nil {
			t.Fatal(shell, err)
		}
	}

	ember.StartupShell = "bash -c injected"
	if err := ember.Validate(); err == nil {
		t.Fatal("unsupported startup shell accepted")
	}
}
