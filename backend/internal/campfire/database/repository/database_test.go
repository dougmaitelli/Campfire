package repository_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each PostgreSQL test gets its own schema, including when opening a second
// connection or restarting Campfire. The default suite continues to use SQLite.
func testDatabaseSource(t *testing.T) string {
	t.Helper()

	source := os.Getenv("CAMPFIRE_TEST_POSTGRES")
	if source == "" {
		return filepath.Join(t.TempDir(), "panel.db")
	}

	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		t.Fatal("CAMPFIRE_TEST_POSTGRES must be a PostgreSQL URL")
	}

	admin, err := sql.Open("pgx", source)
	if err != nil {
		t.Fatal("invalid PostgreSQL test connection")
	}

	schema := "campfire_test_" + strings.ToLower(rand.Text())
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		_ = admin.Close()

		t.Fatal("create isolated test schema:", err)
	}

	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error("remove isolated test schema:", err)
		}

		_ = admin.Close()
	})

	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()

	return parsed.String()
}
