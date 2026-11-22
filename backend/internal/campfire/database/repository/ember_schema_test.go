package repository_test

import (
	"encoding/json"
	"testing"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func TestEmberReadsNormalizeWithoutWritingAndSavesPersistCurrentModel(t *testing.T) {
	db, _, _ := authStore(t)
	store := repository.NewEmberRepository(db.ORM())
	embers := service.NewEmberService(store)

	legacy := model.Ember{ID: "legacy", Schema: 1, Name: "Game", Images: map[string]string{"default": "alpine:latest"}, Startup: "sleep 60", WorkDir: "/home/container", User: "1000:1000", RequiredPorts: []model.RequiredPort{{Container: 25565, Protocol: model.NetworkTCP}}}
	if err := store.Create(t.Context(), legacy); err != nil {
		t.Fatal(err)
	}

	read, err := embers.Get(t.Context(), legacy.ID)
	if err != nil || read.Schema != model.CurrentEmberSchema || !read.RequiredPorts[0].Primary {
		t.Fatal("reader must expose the normalized current model", read, err)
	}

	listed, err := embers.List(t.Context())
	if err != nil || len(listed) != 1 || listed[0].Schema != model.CurrentEmberSchema || !listed[0].RequiredPorts[0].Primary {
		t.Fatal("list reader must normalize too", listed, err)
	}

	var raw string
	if err := db.ORM().Table("embers").Select("definition").Where("id = ?", legacy.ID).Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}

	var stored model.Ember
	if err := json.Unmarshal([]byte(raw), &stored); err != nil || stored.Schema != 1 || stored.RequiredPorts[0].Primary {
		t.Fatal("reading must not rewrite the stored definition", stored, err)
	}

	for _, operation := range []string{"import", "replace"} {
		t.Run(operation, func(t *testing.T) {
			var (
				saved model.Ember
				err   error
			)
			if operation == "import" {
				saved, err = embers.Import(t.Context(), legacy)
			} else {
				edited := read
				edited.Description = "Edited metadata"
				saved, err = embers.Replace(t.Context(), legacy.ID, edited)
			}

			if err != nil {
				t.Fatal(err)
			}

			persisted, err := store.Get(t.Context(), saved.ID)
			if err != nil || saved.Schema != model.CurrentEmberSchema || persisted.Schema != model.CurrentEmberSchema {
				t.Fatal("write did not normalize schema", saved, persisted, err)
			}

			if primary, ok := saved.PrimaryPort(); !ok || primary.Container != 25565 || !persisted.RequiredPorts[0].Primary {
				t.Fatal("normalization did not retain the older first-port default", saved)
			}

			if !saved.SupportsConsoleCommands() {
				t.Fatal("normalization changed console capability default")
			}
		})
	}

	for _, schema := range []int{0, 99} {
		invalid := legacy

		invalid.Schema = schema
		if _, err := embers.Import(t.Context(), invalid); err == nil {
			t.Fatal("invalid schema was silently upgraded")
		}

		if _, err := embers.Replace(t.Context(), legacy.ID, invalid); err == nil {
			t.Fatal("invalid replacement schema was silently upgraded")
		}
	}
}
