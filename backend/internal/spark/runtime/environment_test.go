package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestEnvironmentWithoutPublishedPorts(t *testing.T) {
	d := runtimeFixture()
	d.Configuration.Ports = nil
	d.Configuration.Variables = map[string]string{"SERVER_PORT": "12345"}
	if got := environment(d)["SERVER_PORT"]; got != "12345" {
		t.Fatalf("configured internal port = %q", got)
	}
	d.Ember.RequiredPorts = []model.RequiredPort{{Container: 25565, Protocol: "tcp"}}
	if got := environment(d)["SERVER_PORT"]; got != "25565" {
		t.Fatalf("ember internal port = %q", got)
	}
}

func TestResourcePlaceholdersUseEnforcedLimits(t *testing.T) {
	d := runtimeFixture()
	d.Configuration.CPULimit = 1.25
	d.Configuration.DiskMB = 4096
	d.Configuration.Variables["SERVER_CPU"] = "999"

	env := environment(d)
	for name, want := range map[string]string{"SERVER_CPU": "125", "SERVER_THREADS": "", "SERVER_DISK": "4096", "SERVER_SWAP": "0"} {
		if env[name] != want {
			t.Fatalf("%s = %q; want %q", name, env[name], want)
		}
	}

	d.Ember.Configs = []model.ConfigFile{{Path: "resources.properties", Parser: "properties", Create: true, Values: map[string]string{
		"cpu": "{{SERVER_CPU}}", "threads": "{{SERVER_THREADS}}", "disk": "{{SERVER_DISK}}", "swap": "{{SERVER_SWAP}}",
	}}}

	directory := t.TempDir()
	if err := configure(directory, d); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(directory, "resources.properties"))
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if strings.Join(lines, "\n") != "cpu=125\ndisk=4096\nswap=0\nthreads=" {
		t.Fatalf("resource configuration: %q", data)
	}
}
