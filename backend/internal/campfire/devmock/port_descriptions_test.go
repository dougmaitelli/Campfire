//go:build !production

package devmock

import (
	"encoding/json"
	"strings"
	"testing"

	"campfire/internal/campfire/database/repository"
)

func TestServerPortDescriptionsComeFromEmberWithoutDuplicatingConfiguration(t *testing.T) {
	_, db, _, servers := fixture(t)
	store := repository.NewServerRepository(db.ORM())

	server := servers[0]
	if server.PortDescriptions["25565/tcp"] != "Main" {
		t.Fatal("mock port description missing", server.PortDescriptions)
	}

	if server.EmberArtURL != "/game-art/mock-block.svg" {
		t.Fatal("mock game art missing", server.EmberArtURL)
	}

	embers := repository.NewEmberRepository(db.ORM())

	ember, err := embers.Get(t.Context(), server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	ember.RequiredPorts[0].Description = "Game traffic"

	ember.ArtURL = "https://cdn.example.test/new-art.png"
	if err := embers.Replace(t.Context(), ember); err != nil {
		t.Fatal(err)
	}

	updated, err := store.Get(t.Context(), server.ID)
	if err != nil || updated.PortDescriptions["25565/tcp"] != "Game traffic" {
		t.Fatal("server used a stale copy of its port descriptions", updated.PortDescriptions, err)
	}

	if updated.EmberArtURL != ember.ArtURL {
		t.Fatal("server used stale game art", updated.EmberArtURL)
	}

	configuration, err := json.Marshal(updated.ServerConfiguration)
	if err != nil || strings.Contains(string(configuration), "Game traffic") || strings.Contains(string(configuration), ember.ArtURL) {
		t.Fatal("display descriptions leaked into saved server configuration", string(configuration), err)
	}
}
