package model

import "testing"

func TestContainerRevisionExcludesSettingsThatApplyWithoutRestart(t *testing.T) {
	d := Deployment{Configuration: RuntimeConfiguration{Image: "alpine", MemoryMB: 128}, Ember: RuntimeDefinition{Startup: "sleep 3600", WorkDir: "/data", User: "1000"}}
	before := ContainerRevision(d)
	d.Ember.Installer = &Installer{Script: "echo updated"}
	d.Ember.Stop = "SIGTERM"
	d.Ember.Ready = "ready"
	d.Ember.LogAlerts = []LogAlert{{Title: "Updated alert"}}
	d.Ember.StartupShell = "sh"
	d.Configuration.Variables = map[string]string{}
	d.Configuration.Ports = []ServerPort{}

	d.Ember.Configs = []ConfigFile{}
	if ContainerRevision(d) != before {
		t.Fatal("non-container settings requested a restart")
	}

	d.Configuration.MemoryMB++
	if ContainerRevision(d) == before {
		t.Fatal("container settings did not request a restart")
	}
}
