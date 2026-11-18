package database_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

// This is the released composite-key schema, not the new Backup model.
type legacyBackup struct {
	DisplayName    string           `gorm:"type:text;not null;default:''"`
	Pinned         bool             `gorm:"not null;default:false"`
	Portable       bool             `gorm:"not null;default:false"`
	Mode           model.BackupMode `gorm:"type:text;not null;default:'full'"`
	ServerID       string           `gorm:"type:text;primaryKey"`
	Name           string           `gorm:"type:text;primaryKey"`
	Size           int64            `gorm:"not null"`
	CreatedAt      int64            `gorm:"not null;autoCreateTime:false"`
	Remote         bool             `gorm:"not null"`
	Local          bool             `gorm:"not null"`
	StoreID        string           `gorm:"type:text;not null;default:''"`
	Key            string           `gorm:"type:text;not null"`
	Version        string           `gorm:"type:text;not null;default:''"`
	Digest         string           `gorm:"type:text;not null;default:''"`
	Deleting       bool             `gorm:"not null;default:false"`
	LocalDeleting  bool             `gorm:"not null;default:false"`
	RemoteDeleting bool             `gorm:"not null;default:false"`
	NoRemote       bool             `gorm:"not null;default:false"`
	Error          string           `gorm:"type:text;not null;default:''"`
}

func (legacyBackup) TableName() string { return "backups" }

func TestIndependentBackupMigrationPreservesArchives(t *testing.T) {
	for _, marker := range []int{1, 2} {
		t.Run(fmt.Sprint(marker), func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "panel.db")
			// The optional database is dedicated to this test run, never production.
			if configured := os.Getenv("CAMPFIRE_TEST_MIGRATION_POSTGRES"); configured != "" {
				source = configured
			}

			db, err := database.Open(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}

			spark := database.Spark{ID: "migration-spark", Name: "migration spark", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
			ember := database.Ember{ID: "migration-ember", Name: "migration ember", CreatedAt: spark.CreatedAt}

			server := database.Server{ID: "migration-server", Name: "migration server", SparkID: spark.ID, EmberID: ember.ID, CreatedAt: spark.CreatedAt}
			for _, value := range []any{&spark, &ember, &server} {
				if err := db.ORM().Omit("Spark", "Ember").Create(value).Error; err != nil {
					t.Fatal(err)
				}
			}

			if err := db.ORM().Migrator().DropTable(&database.Backup{}); err != nil {
				t.Fatal(err)
			}

			if err := db.ORM().Migrator().CreateTable(&legacyBackup{}); err != nil {
				t.Fatal(err)
			}

			for _, id := range []string{server.ID, "already-deleted"} {
				row := legacyBackup{ServerID: id, Name: "same.tar.gz", DisplayName: "Keep me", Pinned: true, Portable: true, Mode: model.BackupModeFull, Local: true, Remote: true, Key: id + "/opaque-key", StoreID: "store", Version: "immutable-version", Digest: "checksum", Size: 123, CreatedAt: 42}
				if id == "already-deleted" {
					row.Deleting, row.RemoteDeleting = true, true
					row.Error = "old cleanup failed"
				}

				if err := db.ORM().Create(&row).Error; err != nil {
					t.Fatal(err)
				}
			}

			if err := db.ORM().Exec("ALTER TABLE servers DROP COLUMN deletion_pending").Error; err != nil {
				t.Fatal(err)
			}

			if err := db.ORM().Exec("UPDATE schema_migrations SET version = ?", marker).Error; err != nil {
				t.Fatal(err)
			}

			_ = db.Close()

			reopened, err := database.Open(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}

			var migratedServer database.Server
			if err := reopened.ORM().Where("id = ?", server.ID).Take(&migratedServer).Error; err != nil || migratedServer.DeletionPending {
				t.Fatal("incorrect deletion migration", migratedServer, err)
			}

			var rows []database.Backup
			if err := reopened.ORM().Order("key").Find(&rows).Error; err != nil || len(rows) != 2 {
				t.Fatal(rows, err)
			}

			ids := make(map[string]bool)
			for _, row := range rows {
				if row.ID == "" || ids[row.ID] || row.Name != "same.tar.gz" || row.DisplayName != "Keep me" || !row.Pinned || !row.Local || !row.Remote || row.Size != 123 || row.CreatedAt != 42 || row.Digest != "checksum" || row.Version != "immutable-version" || row.StoreID != "store" {
					t.Fatal("migration changed archive", row)
				}

				if row.ServerID == "" && (row.Deleting || row.RemoteDeleting || row.LocalDeleting || row.Error != "") {
					t.Fatal("orphan inherited obsolete cleanup", row)
				}

				ids[row.ID] = true
				if row.ServerID != "" && (row.ServerID != server.ID || row.LocalStore != "spark:"+spark.ID || row.LocalKey != server.ID+"/same.tar.gz") {
					t.Fatal("storage locator lost", row)
				}
			}

			var unlinked int64
			if err := reopened.ORM().Model(&database.Backup{}).Where("server_id IS NULL").Count(&unlinked).Error; err != nil || unlinked != 1 {
				t.Fatal("association is not nullable", unlinked, err)
			}
			// Equal filenames are allowed for independent archives, never identities.
			if err := reopened.ORM().Create(&database.Backup{Name: "same.tar.gz", Local: true, Key: "new-key"}).Error; err != nil {
				t.Fatal(err)
			}

			_ = reopened.Close()

			reopened, err = database.Open(t.Context(), source)
			if err != nil {
				t.Fatal("migration not idempotent", err)
			}
			defer func() { _ = reopened.Close() }()

			var repeated []database.Backup
			if err := reopened.ORM().Find(&repeated).Error; err != nil || len(repeated) != 3 {
				t.Fatal(repeated, err)
			}

			for _, row := range repeated {
				if row.Key != "new-key" && !ids[row.ID] {
					t.Fatal("reopen changed archive identity")
				}
			}

			if os.Getenv("CAMPFIRE_TEST_MIGRATION_POSTGRES") != "" {
				// Reset this dedicated database's fixture rows for the second marker case.
				for _, value := range []any{&database.Backup{}, &database.Server{}, &database.Ember{}, &database.Spark{}} {
					if err := reopened.ORM().Where("1 = 1").Delete(value).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}
