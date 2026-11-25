package repository_test

import (
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/integrations"
	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"
)

func TestSchemaSixAddsOverridesWithoutFreezingSparkDefaults(t *testing.T) {
	source := testDatabaseSource(t)

	db, err := database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	created := time.Now().UTC().Format(time.RFC3339Nano)
	if err := db.ORM().Create(&database.Spark{ID: "node", Name: "Node", GameAddress: "games.example.test", CreatedAt: created}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Create(&database.Ember{ID: "game", Name: "Game", Definition: model.Ember{ID: "game", Name: "Game"}}).Error; err != nil {
		t.Fatal(err)
	}

	for i, id := range []string{"first", "second"} {
		row := database.Server{ID: id, Name: id, SparkID: "node", EmberID: "game", Variables: map[string]string{}, Ports: []model.ServerPort{{HostPort: 25565 + i, ContainerPort: 25565, Protocol: model.NetworkTCP}}, CreatedAt: created}
		if err := db.ORM().Omit("Spark", "Ember", "Owner").Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	if _, err := repository.NewStatusPageRepository(db.ORM()).Save(t.Context(), "", statuspage.Input{Title: "Community", Slug: "community", Entries: []statuspage.Selection{{ServerID: "first", Name: "First"}, {ServerID: "second", Name: "Second"}}}); err != nil {
		t.Fatal(err)
	}

	for _, sql := range []string{"ALTER TABLE servers DROP COLUMN game_address", "UPDATE schema_migrations SET version = 6"} {
		if err := db.ORM().Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var version int
	if err := db.ORM().Table("schema_migrations").Select("version").Scan(&version).Error; err != nil || version != 12 {
		t.Fatal(version, err)
	}

	var servers []database.Server
	if err := db.ORM().Find(&servers).Error; err != nil {
		t.Fatal(err)
	}

	for _, server := range servers {
		if server.GameAddress != "" {
			t.Fatal("migration froze an inherited default", server)
		}
	}

	assertAddresses := func(first, second string) {
		t.Helper()

		_, targets, err := repository.NewStatusPageRepository(db.ORM()).Published(t.Context(), "community")
		if err != nil || len(targets) != 2 || targets[0].GameAddress != first || targets[1].GameAddress != second {
			t.Fatal(targets, err)
		}

		for _, target := range targets {
			server, err := repository.NewServerRepository(db.ORM()).Get(t.Context(), target.ServerID)
			if err != nil {
				t.Fatal(err)
			}

			card := integrations.BuildServerCard(server, model.RuntimeStateStopped, nil, integrations.CardSettings{}, nil)
			if target.GameAddress == "" && len(card.Addresses) == 0 {
				continue
			}

			if len(card.Addresses) != 1 || card.Addresses[0] != target.GameAddress {
				t.Fatal("public page and card disagree", card, target)
			}
		}
	}
	assertAddresses("", "") // A definition without ports cannot supply an address.

	older := model.Ember{ID: "game", Name: "Game", Schema: 1, RequiredPorts: []model.RequiredPort{{Container: 25565, Protocol: model.NetworkTCP}}}
	if err := db.ORM().Model(&database.Ember{}).Where("id = ?", "game").Updates(&database.Ember{Definition: older}).Error; err != nil {
		t.Fatal(err)
	}

	assertAddresses("games.example.test:25565", "games.example.test:25566")

	older.Schema = model.CurrentEmberSchema
	if err := db.ORM().Model(&database.Ember{}).Where("id = ?", "game").Updates(&database.Ember{Definition: older}).Error; err != nil {
		t.Fatal(err)
	}

	assertAddresses("", "") // Current definitions preserve an intentional no-primary choice.

	definition := model.Ember{ID: "game", Name: "Game", Schema: 2, RequiredPorts: []model.RequiredPort{{Container: 25565, Protocol: model.NetworkTCP, Primary: true}}}
	if err := db.ORM().Model(&database.Ember{}).Where("id = ?", "game").Updates(&database.Ember{Definition: definition}).Error; err != nil {
		t.Fatal(err)
	}

	assertAddresses("games.example.test:25565", "games.example.test:25566")

	if err := db.ORM().Model(&database.Spark{}).Where("id = ?", "node").Update("game_address", "2001:db8::1").Error; err != nil {
		t.Fatal(err)
	}

	assertAddresses("[2001:db8::1]:25565", "[2001:db8::1]:25566")

	const override = "https://Play.Example.test:32100/Join"
	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "first").Update("game_address", override).Error; err != nil {
		t.Fatal(err)
	}

	assertAddresses(override, "[2001:db8::1]:25566")

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	assertAddresses(override, "[2001:db8::1]:25566")

	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "first").Update("game_address", "").Error; err != nil {
		t.Fatal(err)
	}

	assertAddresses("[2001:db8::1]:25565", "[2001:db8::1]:25566")

	// Old defaults may predate stricter settings validation; reads must still
	// suppress them consistently without overriding explicit server text.
	if err := db.ORM().Model(&database.Spark{}).Where("id = ?", "node").Update("game_address", "::").Error; err != nil {
		t.Fatal(err)
	}

	assertAddresses("", "")

	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "first").Update("game_address", override).Error; err != nil {
		t.Fatal(err)
	}

	assertAddresses(override, "")

	if err := db.ORM().Where("id = ?", "node").Delete(&database.Spark{}).Error; err == nil {
		t.Fatal("migration lost foreign key constraints")
	}
}
