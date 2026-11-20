package repository_test

import (
	"context"
	"errors"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/statuspage"

	"gorm.io/gorm"
)

func TestStatusPagePublicationUsesOneReadForTitleAndMembership(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewStatusPageRepository(db.ORM())

	page, err := store.Save(t.Context(), "", statuspage.Input{Title: "First community", Slug: "community", Entries: []statuspage.Selection{{ServerID: "first", Name: "First game"}}})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	reads := 0

	count := func(tx *gorm.DB) {
		if !tx.DryRun && tx.Statement.Context == ctx {
			reads++
		}
	}
	if err := db.ORM().Callback().Query().Before("gorm:query").Register("test:publication-query", count); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Callback().Row().Before("gorm:row").Register("test:publication-row", count); err != nil {
		t.Fatal(err)
	}

	assertPublication := func(title, server string) {
		t.Helper()

		reads = 0

		got, targets, err := store.Published(ctx, "community")
		if err != nil || got != title || len(targets) != 1 || targets[0].ServerID != server {
			t.Fatal(got, targets, err)
		}

		if reads != 1 {
			t.Fatalf("publication requires one database snapshot, got %d reads", reads)
		}
	}
	assertPublication("First community", "first")

	if _, err := store.Save(t.Context(), page.ID, statuspage.Input{Title: "Second community", Slug: "community", Entries: []statuspage.Selection{{ServerID: "second", Name: "Second game"}}}); err != nil {
		t.Fatal(err)
	}

	assertPublication("Second community", "second")
}

func TestStatusPageMembershipOrderingAtomicityAndDeletion(t *testing.T) {
	db, _ := integrationEventStore(t)
	store := repository.NewStatusPageRepository(db.ORM())
	input := statuspage.Input{Title: "Public", Slug: "community", Entries: []statuspage.Selection{{ServerID: "second", Name: "Public second"}, {ServerID: "first", Name: "Public first"}}}

	page, err := store.Save(t.Context(), "", input)
	if err != nil {
		t.Fatal(err)
	}

	title, targets, err := store.Published(t.Context(), "community")
	if err != nil || title != "Public" || len(targets) != 2 || targets[0].ServerID != "second" || targets[1].ServerID != "first" {
		t.Fatal(title, targets, err)
	}

	firstID := targets[1].ID

	input.Entries = []statuspage.Selection{{ServerID: "first", Name: "Renamed"}}
	if _, err := store.Save(t.Context(), page.ID, input); err != nil {
		t.Fatal(err)
	}

	_, targets, err = store.Published(t.Context(), "community")
	if err != nil || len(targets) != 1 || targets[0].ID != firstID || targets[0].Name != "Renamed" {
		t.Fatal(targets, err)
	}

	input.Entries = []statuspage.Selection{{ServerID: "missing", Name: "Missing"}}
	if _, err := store.Save(t.Context(), page.ID, input); !errors.Is(err, statuspage.ErrInput) {
		t.Fatal(err)
	}

	_, targets, err = store.Published(t.Context(), "community")
	if err != nil || len(targets) != 1 || targets[0].ID != firstID {
		t.Fatal("invalid edit changed membership", targets, err)
	}

	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "first").Update("deletion_pending", true).Error; err != nil {
		t.Fatal(err)
	}

	_, targets, err = store.Published(t.Context(), "community")
	if err != nil || len(targets) != 0 {
		t.Fatal("deleting server remained public", targets, err)
	}

	if err := db.ORM().Where("id = ?", "first").Delete(&database.Server{}).Error; err != nil {
		t.Fatal(err)
	}

	pages, err := store.List(t.Context())
	if err != nil || len(pages) != 1 || len(pages[0].Entries) != 0 {
		t.Fatal("server deletion did not remove membership", pages, err)
	}

	input.Entries = []statuspage.Selection{{ServerID: "second", Name: "Remaining game"}}
	if _, err := store.Save(t.Context(), page.ID, input); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(t.Context(), page.ID); err != nil {
		t.Fatal(err)
	}

	var entries, servers int64
	if err := db.ORM().Model(&database.StatusPageEntry{}).Count(&entries).Error; err != nil || entries != 0 {
		t.Fatal("page deletion retained memberships", entries, err)
	}

	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "second").Count(&servers).Error; err != nil || servers != 1 {
		t.Fatal("page deletion affected its server", servers, err)
	}

	if _, _, err := store.Published(t.Context(), "community"); !errors.Is(err, statuspage.ErrNotFound) {
		t.Fatal(err)
	}
}
