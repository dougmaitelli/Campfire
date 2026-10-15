// Package database owns SQLite setup and schema migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Database owns the ORM and its underlying connection pool.
type Database struct {
	orm *gorm.DB
	sql *sql.DB
}

// Open opens SQLite and applies versioned ORM migrations transactionally.
func Open(ctx context.Context, path string) (*Database, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	dsn := (&url.URL{Scheme: "file", Path: absolute}).String() + "?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL&_txlock=immediate"

	orm, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	pool, err := orm.DB()
	if err != nil {
		return nil, fmt.Errorf("access database pool: %w", err)
	}

	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)

	db := &Database{orm: orm, sql: pool}
	if err := migrate(orm.WithContext(ctx)); err != nil {
		_ = pool.Close()
		return nil, err
	}

	return db, nil
}

// ORM returns the shared ORM for repositories.
func (d *Database) ORM() *gorm.DB {
	return d.orm
}

// Close releases the underlying connection pool.
func (d *Database) Close() error {
	return d.sql.Close()
}

func migrate(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&schemaMigration{}); err != nil {
			return fmt.Errorf("initialize migrations: %w", err)
		}

		var version int
		if err := tx.Model(&schemaMigration{}).Select("COALESCE(MAX(version), 0)").Scan(&version).Error; err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}

		if version > 9 {
			return fmt.Errorf("database schema version %d is newer than supported version 9", version)
		}

		if version == 0 {
			if err := tx.AutoMigrate(&Spark{}); err != nil {
				return fmt.Errorf("apply Spark schema: %w", err)
			}

			if err := tx.Create(&schemaMigration{Version: 3}).Error; err != nil {
				return fmt.Errorf("record migration: %w", err)
			}
		}

		if version == 1 {
			for _, column := range []string{"Description", "Disabled"} {
				if err := tx.Migrator().AddColumn(&Spark{}, column); err != nil {
					return fmt.Errorf("add Spark %s: %w", column, err)
				}
			}

			if err := tx.Create(&schemaMigration{Version: 2}).Error; err != nil {
				return fmt.Errorf("record migration: %w", err)
			}
		}

		if version == 1 || version == 2 {
			for _, column := range []string{"GameAddress", "CPULimit", "MemoryMB", "DiskMB", "PortAllocations"} {
				if err := tx.Migrator().AddColumn(&Spark{}, column); err != nil {
					return fmt.Errorf("add Spark %s: %w", column, err)
				}
			}

			if err := tx.Create(&schemaMigration{Version: 3}).Error; err != nil {
				return fmt.Errorf("record migration: %w", err)
			}
		}

		if version < 4 {
			if err := tx.Migrator().CreateTable(&Ember{}, &Server{}); err != nil {
				return fmt.Errorf("apply server schema: %w", err)
			}

			if err := tx.Create(&schemaMigration{Version: 4}).Error; err != nil {
				return fmt.Errorf("record migration: %w", err)
			}
		}

		if version < 5 {
			if err := tx.Migrator().CreateTable(&SparkEnrollment{}, &SparkIdentity{}); err != nil {
				return fmt.Errorf("apply Spark enrollment schema: %w", err)
			}

			if err := tx.Create(&schemaMigration{Version: 5}).Error; err != nil {
				return fmt.Errorf("record migration: %w", err)
			}
		}

		if version < 6 {
			if err := tx.Migrator().CreateTable(&AdminCredential{}, &AdminSession{}); err != nil {
				return fmt.Errorf("apply administrator schema: %w", err)
			}

			if err := tx.Create(&schemaMigration{Version: 6}).Error; err != nil {
				return fmt.Errorf("record migration: %w", err)
			}
		}

		if version < 7 {
			if err := migrateAccess(tx); err != nil {
				return fmt.Errorf("apply account authorization schema: %w", err)
			}

			if err := tx.Create(&schemaMigration{Version: 7}).Error; err != nil {
				return fmt.Errorf("record migration: %w", err)
			}
		}

		if version < 8 {
			if err := tx.Migrator().CreateTable(&OIDCIdentity{}); err != nil {
				return fmt.Errorf("apply OIDC identity schema: %w", err)
			}

			if !tx.Migrator().HasColumn(&UserSession{}, "Provider") {
				if err := tx.Migrator().AddColumn(&UserSession{}, "Provider"); err != nil {
					return fmt.Errorf("apply session provider schema: %w", err)
				}
			}

			if err := tx.Create(&schemaMigration{Version: 8}).Error; err != nil {
				return fmt.Errorf("record migration: %w", err)
			}
		}

		if version < 9 {
			if err := tx.Migrator().CreateTable(&Schedule{}, &ScheduleRun{}, &AuditEvent{}); err != nil {
				return fmt.Errorf("apply schedules and audit schema: %w", err)
			}

			if err := tx.Create(&schemaMigration{Version: 9}).Error; err != nil {
				return fmt.Errorf("record migration: %w", err)
			}
		}

		// Keep the current development schema in sync without introducing a new
		// release migration for features added before the first release.
		if !tx.Migrator().HasTable(&APIToken{}) {
			if err := tx.Migrator().CreateTable(&APIToken{}); err != nil {
				return fmt.Errorf("create API tokens: %w", err)
			}
		}

		current := tx.Session(&gorm.Session{})
		for _, model := range []any{&Notification{}, &NotificationRecipient{}, &NotificationDelivery{}, &AlertState{}} {
			if !tx.Migrator().HasTable(model) {
				if err := tx.Migrator().CreateTable(model); err != nil {
					return fmt.Errorf("create notification schema: %w", err)
				}
			}
		}

		current.IgnoreRelationshipsWhenMigrating = true
		if err := current.AutoMigrate(&SparkIdentity{}, &ServerTransfer{}, &BackupPolicy{}, &Backup{}, &AuditEvent{}, &ScheduleRun{}); err != nil {
			return fmt.Errorf("apply Spark renewal and transfer schema: %w", err)
		}

		return nil
	})
}
