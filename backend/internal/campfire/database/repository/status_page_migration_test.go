package repository_test

import (
	"testing"

	"campfire/internal/campfire/database"
)

func TestSchemaFiveAddsStatusPagesOnUpgrade(t *testing.T) {
	path := testDatabaseSource(t)

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropTable(&database.StatusPageEntry{}, &database.StatusPage{}); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Table("schema_migrations").Where("version = ?", 6).Update("version", 5).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	for _, row := range []any{&database.StatusPage{}, &database.StatusPageEntry{}} {
		if !db.ORM().Migrator().HasTable(row) {
			t.Fatal("missing new status page table")
		}
	}

	var version int
	if err := db.ORM().Table("schema_migrations").Select("version").Scan(&version).Error; err != nil || version != 6 {
		t.Fatal(version, err)
	}
}
