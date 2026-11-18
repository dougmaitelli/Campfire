package repository_test

import (
	"errors"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestLinkedArchiveStorageFollowsCurrentPlacement(t *testing.T) {
	db, _, _ := authStore(t)
	for _, row := range []any{
		&database.Spark{ID: "current", Name: "Current node"},
		&database.Spark{ID: "previous", Name: "Previous node"},
		&database.Ember{ID: "ember", Name: "Game"},
		&database.Server{ID: "live", Name: "Live server", SparkID: "current", EmberID: "ember"},
		&database.Backup{ServerID: "live", Name: "snapshot.tar.gz", Local: true, LocalStore: "spark:previous", LocalKey: "live/snapshot.tar.gz"},
	} {
		if err := db.ORM().Omit("Spark", "Ember", "Owner").Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	rows, err := repository.NewBackupRepository(db.ORM()).ListAll(t.Context())
	if err != nil || len(rows) != 1 || rows[0].LocalSparkID != "current" || rows[0].LocalSparkName != "Current node" || rows[0].ServerName != "Live server" {
		t.Fatal(rows, err)
	}
}

func TestIndependentArchiveProtectsOnlyItsStorageSpark(t *testing.T) {
	db, _, _ := authStore(t)
	ctx := t.Context()

	sparks := repository.NewSparkRepository(db.ORM())
	for _, id := range []string{"storage", "other"} {
		if err := sparks.Create(ctx, model.Spark{ID: id, Name: id, CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}

	archive := database.Backup{Name: "snapshot.tar.gz", Local: true, Remote: true, LocalStore: "spark:storage", LocalKey: "namespace/snapshot.tar.gz"}
	if err := db.ORM().Create(&archive).Error; err != nil {
		t.Fatal(err)
	}

	backups := repository.NewBackupRepository(db.ORM())

	catalog, err := backups.ListAll(ctx)
	if err != nil || len(catalog) != 1 || catalog[0].LocalSparkID != "storage" || catalog[0].LocalSparkName != "storage" || catalog[0].ServerID != "" {
		t.Fatal("current storage not identifiable", catalog, err)
	}

	if err := sparks.Delete(ctx, "other"); err != nil {
		t.Fatal(err)
	}

	if err := sparks.Delete(ctx, "storage"); !errors.Is(err, repository.ErrSparkArchives) {
		t.Fatalf("revoked archive storage identity: %v", err)
	}
	// Once its local copy has been explicitly removed, a remote-only archive does
	// not prevent removing an unrelated storage backend or lose its own identity.
	if err := backups.UpdateArchive(ctx, archive.ID, map[string]any{"local": false}); err != nil {
		t.Fatal(err)
	}

	catalog, err = backups.ListAll(ctx)
	if err != nil || len(catalog) != 1 || catalog[0].LocalSparkID != "" || catalog[0].LocalSparkName != "" {
		t.Fatal("location outlived its local copy", catalog, err)
	}

	if err := sparks.Delete(ctx, "storage"); err != nil {
		t.Fatal(err)
	}

	saved, err := backups.ByID(ctx, archive.ID)
	if err != nil || !saved.Remote || saved.ServerID != "" {
		t.Fatalf("archive changed with Spark removal: %+v %v", saved, err)
	}
}
