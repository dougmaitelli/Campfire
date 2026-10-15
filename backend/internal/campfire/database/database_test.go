package database_test

import (
	"context"
	"path/filepath"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestRejectNewerSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().WithContext(ctx).Exec("INSERT INTO schema_migrations(version) VALUES (7)").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(ctx, path)
	if err == nil {
		_ = reopened.Close()

		t.Fatal("opened unsupported future schema")
	}
}

func TestAuthenticationMigrationPreservesPopulatedVersionFive(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")

	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	for _, record := range []any{
		&database.Spark{ID: "node", Name: "Preserved node", Kind: "remote", CreatedAt: "2026-10-14T00:00:00Z"},
		&database.Ember{ID: "ember", Name: "Preserved ember", Definition: model.Ember{Schema: 1, Name: "Preserved ember"}, CreatedAt: "2026-10-14T00:00:00Z"},
		&database.Server{ID: "server", Name: "Preserved server", SparkID: "node", EmberID: "ember", Image: "alpine:3.22", Variables: map[string]string{"KEEP": "value"}, CPULimit: 1, MemoryMB: 128, DiskMB: 1024, Ports: []model.ServerPort{{BindIP: "127.0.0.1", HostPort: 25565, ContainerPort: 25565, Protocol: "tcp"}}, CreatedAt: "2026-10-14T00:00:00Z"},
		&database.SparkIdentity{SparkID: "node", CertificateHash: "preserved-certificate"},
	} {
		if err := db.ORM().Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Recreate the pre-auth schema by removing only the new tables and migration.
	if err := db.ORM().Migrator().DropTable(&database.AdminSession{}, &database.AdminCredential{}); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Exec("DELETE FROM schema_migrations WHERE version = 6").Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()

	var server database.Server
	if err := reopened.ORM().Preload("Spark").Preload("Ember").Take(&server, "id = ?", "server").Error; err != nil {
		t.Fatal(err)
	}

	if server.Name != "Preserved server" || server.Variables["KEEP"] != "value" || server.Spark.Name != "Preserved node" || server.Ember.Definition.Name != "Preserved ember" || len(server.Ports) != 1 || server.Ports[0].HostPort != 25565 {
		t.Fatal("authentication migration changed existing game configuration")
	}

	var identity database.SparkIdentity
	if err := reopened.ORM().Take(&identity, "spark_id = ?", "node").Error; err != nil || identity.CertificateHash != "preserved-certificate" {
		t.Fatal("authentication migration changed node identity", err)
	}

	if !reopened.ORM().Migrator().HasTable(&database.AdminCredential{}) || !reopened.ORM().Migrator().HasTable(&database.AdminSession{}) {
		t.Fatal("authentication tables were not created")
	}
}
