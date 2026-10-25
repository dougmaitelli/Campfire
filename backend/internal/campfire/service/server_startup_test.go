package service

import (
	"maps"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestServerStartupSelectionSurvivesUpdatesAndRejectsRemovedKeys(t *testing.T) {
	f := newTransferFixture(t)
	d := f.service.Deployment

	ember, err := d.Embers.Get(t.Context(), f.server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	ember.Startup = ""

	ember.StartupCommands = map[string]string{"Normal": "sleep 3600", "Debug": "echo debug; sleep 3600"}
	if _, err := d.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	input := f.server.ServerConfiguration
	if _, err := d.Servers.ApplyEmber(t.Context(), f.server.ID, ember.ID, input); err == nil {
		t.Fatal("multiple commands did not require a selection")
	}

	input.StartupCommand = "Debug"

	server, err := d.Servers.ApplyEmber(t.Context(), f.server.ID, ember.ID, input)
	if err != nil || server.StartupCommand != "Debug" {
		t.Fatal("selection was not persisted", server, err)
	}

	input.StartupCommand = "missing"
	if _, err := d.Servers.Update(t.Context(), server.ID, input); err == nil {
		t.Fatal("invalid selection was saved")
	}

	input = server.ServerConfiguration
	ember.StartupCommands = maps.Clone(ember.StartupCommands)

	ember.StartupCommands["Debug"] = "echo new-debug; sleep 3600"
	if _, err := d.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	server, err = d.Servers.ApplyEmber(t.Context(), server.ID, ember.ID, input)
	if err != nil || server.StartupCommand != "Debug" {
		t.Fatal("changed command text lost its selected key", server, err)
	}

	delete(ember.StartupCommands, "Debug")

	if _, err := d.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	if _, err := d.Servers.ApplyEmber(t.Context(), server.ID, ember.ID, input); err == nil {
		t.Fatal("removed command silently switched variants")
	}

	input.StartupCommand = "Normal"
	if _, err := d.Servers.ApplyEmber(t.Context(), server.ID, ember.ID, input); err != nil {
		t.Fatal("explicit replacement selection failed", err)
	}
}

func TestCreateServerWithoutPortsOrSparkAllocations(t *testing.T) {
	f := newTransferFixture(t)
	d := f.service.Deployment
	ember, err := d.Embers.Import(t.Context(), model.Ember{Schema: 1, Name: "Tunnel game", Images: map[string]string{"Game": "alpine:3.22"}, Startup: "sleep 3600", WorkDir: "/data", User: "1000", RequiredPorts: []model.RequiredPort{{Container: 25565, Protocol: "tcp"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.ORM().Model(&database.Spark{}).Where("id = ?", "target").Update("port_allocations", "[]").Error; err != nil {
		t.Fatal(err)
	}
	input := f.server.ServerConfiguration
	input.Name, input.SparkID, input.EmberID, input.Ports = "Tunnel", "target", ember.ID, nil
	server, err := d.Servers.Create(t.Context(), input)
	if err != nil || server.Ports == nil || len(server.Ports) != 0 {
		t.Fatal("server without published ports was rejected", server, err)
	}
}

func TestCreateServerNormalizesSingleNamedStartup(t *testing.T) {
	f := newTransferFixture(t)
	d := f.service.Deployment
	ember := model.Ember{Schema: 1, Name: "Named game", Images: map[string]string{"Game": "alpine:3.22"}, StartupCommands: map[string]string{"Normal": "sleep 3600"}, WorkDir: "/data", User: "1000"}

	ember, err := d.Embers.Import(t.Context(), ember)
	if err != nil {
		t.Fatal(err)
	}

	input := f.server.ServerConfiguration
	input.Name, input.SparkID, input.EmberID = "Named", "target", ember.ID

	server, err := d.Servers.Create(t.Context(), input)
	if err != nil || server.StartupCommand != "Normal" {
		t.Fatal("single command key was not saved", server, err)
	}
}

func TestServersChooseDifferentCommandsFromSameEmber(t *testing.T) {
	f := newTransferFixture(t)
	d := f.service.Deployment

	ember, err := d.Embers.Import(t.Context(), model.Ember{Schema: 1, Name: "Named game", Images: map[string]string{"Game": "alpine:3.22"}, StartupCommands: map[string]string{"Normal": "sleep 3600", "Debug": "echo debug; sleep 3600"}, WorkDir: "/data", User: "1000"})
	if err != nil {
		t.Fatal(err)
	}

	input := f.server.ServerConfiguration

	input.SparkID, input.EmberID, input.MemoryMB = "target", ember.ID, 64
	for i, command := range []string{"Normal", "Debug"} {
		input.Name, input.StartupCommand = command, command
		input.Ports = []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25565 + i, ContainerPort: 25565, Protocol: "tcp"}}

		server, err := d.Servers.Create(t.Context(), input)
		if err != nil || server.StartupCommand != command {
			t.Fatal("servers did not retain independent selections", server, err)
		}
	}
}
