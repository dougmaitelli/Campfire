package service

import (
	"errors"
	"testing"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestDeletionReviewDoesNotFenceAnUndeployedServer(t *testing.T) {
	f, _, _, _ := backupFixture(t)
	d := f.service.Deployment
	input := f.server.ServerConfiguration
	input.Name, input.SparkID, input.Ports = "Never deployed", "target", nil

	server, err := d.Servers.CreateOwned(t.Context(), input, 1)
	if err != nil {
		t.Fatal(err)
	}

	ember, err := d.Embers.Get(t.Context(), server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	ember.Variables = append(ember.Variables, model.Variable{Name: "NEW_OPTIONAL_VALUE", Editable: true})
	if _, err := d.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	server, err = d.Servers.Get(t.Context(), server.ID)
	if err != nil || !server.EmberReviewRequired {
		t.Fatal("expected review", server, err)
	}

	for range 2 {
		if err := d.Delete(t.Context(), server.ID); !errors.Is(err, ErrInvalidServer) {
			t.Fatal("expected review before deletion", err)
		}

		pending, err := repository.NewServerRepository(f.db.ORM()).DeletionPending(t.Context(), server.ID)
		if err != nil || pending {
			t.Fatal("undispatched deletion fenced review", pending, err)
		}
	}

	if _, err := d.Servers.ApplyEmber(t.Context(), server.ID, ember.ID, ember.ProposedServerConfiguration(server.ServerConfiguration)); err != nil {
		t.Fatal("review could not repair configuration", err)
	}

	if err := d.Delete(t.Context(), server.ID); err != nil {
		t.Fatal("deletion after review failed", err)
	}
	// A deployed server can still be removed without adopting changed Ember values.
	if err := d.Delete(t.Context(), f.server.ID); err != nil {
		t.Fatal("existing deployment unnecessarily required review", err)
	}
}
