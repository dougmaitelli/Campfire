package service

import (
	"errors"
	"testing"

	"campfire/internal/shared/model"
)

func TestConsoleCapabilityRejectsCommandsWithoutBlockingLogs(t *testing.T) {
	f := newTransferFixture(t)
	d := f.service.Deployment

	ember, err := d.Embers.Get(t.Context(), f.server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	ember.ConsoleCommands = new(bool)
	if _, err := d.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	if _, err := d.Console(t.Context(), f.server.ID, "say hello", false); !errors.Is(err, ErrInvalidServer) {
		t.Fatal("unsupported console command was not rejected", err)
	}
	// Scheduled commands use this same dispatch path.
	if _, err := d.dispatchRequest(t.Context(), model.RuntimeRequest{ServerID: f.server.ID, Action: model.ServerActionCommand, Content: "say hello"}); !errors.Is(err, ErrInvalidServer) {
		t.Fatal("scheduled command bypassed capability", err)
	}

	if _, err := d.Console(t.Context(), f.server.ID, "", true); err != nil {
		t.Fatal("console output blocked", err)
	}

	ember.ConsoleCommands = new(bool(true))
	if _, err := d.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	server, err := d.Servers.Get(t.Context(), f.server.ID)
	if err != nil || !server.ConsoleCommands {
		t.Fatal("capability change did not apply immediately", err)
	}
}
