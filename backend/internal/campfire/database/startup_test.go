package database_test

import (
	"path/filepath"
	"testing"

	"campfire/internal/campfire/database"
)

func TestExistingBaselineAddsStartupSelectionColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropColumn(&database.Server{}, "StartupCommand"); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropColumn(&database.Server{}, "StartupOverride"); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Migrator().DropColumn(&database.Server{}, "GameIntegration"); err != nil {
		t.Fatal(err)
	}

	for _, field := range []string{"OIDCNameManaged", "OIDCEmailManaged"} {
		if err := db.ORM().Migrator().DropColumn(&database.User{}, field); err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if !db.ORM().Migrator().HasColumn(&database.Server{}, "StartupCommand") {
		t.Fatal("startup selection column was not added to the existing baseline")
	}

	if !db.ORM().Migrator().HasColumn(&database.Server{}, "StartupOverride") {
		t.Fatal("startup override column was not added to the existing baseline")
	}

	if !db.ORM().Migrator().HasColumn(&database.Server{}, "GameIntegration") {
		t.Fatal("game integration column was not added to the existing baseline")
	}

	for _, field := range []string{"OIDCNameManaged", "OIDCEmailManaged"} {
		if !db.ORM().Migrator().HasColumn(&database.User{}, field) {
			t.Fatal("OIDC profile management column was not added", field)
		}
	}
}

func TestExistingBaselineAddsResourceHistoryTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	tables := []any{&database.ServerResourceBucket{}, &database.SparkResourceBucket{}, &database.ServerResourceSample{}, &database.SparkResourceSample{}}
	if err := db.ORM().Migrator().DropTable(tables...); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	for _, table := range tables {
		if !db.ORM().Migrator().HasTable(table) {
			t.Fatalf("history table missing on upgrade: %T", table)
		}
	}
}
