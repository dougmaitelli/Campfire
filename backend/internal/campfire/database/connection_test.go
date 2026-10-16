package database_test

import (
	"context"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
)

func TestInvalidDatabaseSourcesDoNotExposeCredentials(t *testing.T) {
	for _, source := range []string{
		"", "mysql://user:private-secret@db/campfire",
		"postgres://user:private-secret@db/", "postgresql://user:private-secret@db/campfire#fragment",
		"postgres://user:private-secret@db:invalid/campfire",
		"postgres://user:private-secret@db/campfire?connect_timeout=invalid",
	} {
		db, err := database.Open(t.Context(), source)
		if err == nil {
			_ = db.Close()

			t.Fatal("invalid database source accepted")
		}

		if strings.Contains(err.Error(), "private-secret") || strings.Contains(err.Error(), source) && source != "" {
			t.Fatal("database error exposed connection details")
		}
	}
}

func TestDatabaseConnectionRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	db, err := database.Open(ctx, "postgresql://user:private-secret@127.0.0.1:1/campfire?sslmode=disable")
	if err == nil {
		_ = db.Close()

		t.Fatal("cancelled database connection succeeded")
	}

	if strings.Contains(err.Error(), "private-secret") {
		t.Fatal("connection error exposed password")
	}
}
