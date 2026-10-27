// Package database owns SQLite and PostgreSQL setup and schema migrations.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Database owns the ORM and its underlying connection pool.
type Database struct {
	orm *gorm.DB
	sql *sql.DB
}

const schemaVersion = 1

// Open accepts a SQLite file path or a postgres:// / postgresql:// URL.
// It applies the same transactional schema migrations to either backend.
func Open(ctx context.Context, source string) (*Database, error) {
	driver, err := dialector(source)
	if err != nil {
		return nil, err
	}

	orm, err := gorm.Open(driver, &gorm.Config{
		Logger:               logger.Default.LogMode(logger.Silent),
		TranslateError:       true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		if driver.Name() == "postgres" {
			return nil, errors.New("open PostgreSQL database: invalid connection configuration")
		}

		return nil, fmt.Errorf("open database: %w", err)
	}

	pool, err := orm.DB()
	if err != nil {
		return nil, fmt.Errorf("access database pool: %w", err)
	}

	if driver.Name() == "postgres" {
		if err := registerWriteLocks(orm); err != nil {
			_ = pool.Close()

			return nil, fmt.Errorf("initialize database transactions: %w", err)
		}

		pool.SetMaxOpenConns(10)
		pool.SetMaxIdleConns(2)
		pool.SetConnMaxLifetime(30 * time.Minute)
	} else {
		pool.SetMaxOpenConns(1)
		pool.SetMaxIdleConns(1)
	}

	pingContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := pool.PingContext(pingContext); err != nil {
		_ = pool.Close()

		// Driver errors can contain the connection URL and its password.
		return nil, errors.New("connect to database: connection failed; check database configuration and availability")
	}

	db := &Database{orm: orm, sql: pool}
	if err := migrate(orm.WithContext(ctx)); err != nil {
		_ = pool.Close()
		return nil, err
	}

	return db, nil
}

func dialector(source string) (gorm.Dialector, error) {
	if strings.HasPrefix(source, "postgres://") || strings.HasPrefix(source, "postgresql://") {
		parsed, err := url.Parse(source)
		if err != nil || parsed.Host == "" || parsed.Path == "" || parsed.Path == "/" || parsed.Fragment != "" {
			return nil, errors.New("invalid PostgreSQL URL: expected postgres://USER:PASSWORD@HOST:PORT/DATABASE")
		}

		return postgres.Open(source), nil
	}

	if strings.TrimSpace(source) == "" || strings.Contains(source, "://") {
		return nil, errors.New("database must be a SQLite file path or PostgreSQL URL")
	}

	absolute, err := filepath.Abs(source)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	dsn := (&url.URL{Scheme: "file", Path: absolute}).String() + "?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL&_txlock=immediate"

	return sqlite.Open(dsn), nil
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
	if db.Name() == "sqlite" {
		if err := db.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
			return err
		}
		defer db.Exec("PRAGMA foreign_keys = ON")
	}

	return Transaction(db, func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable(&schemaMigration{}) {
			if err := tx.Migrator().CreateTable(&schemaMigration{}); err != nil {
				return fmt.Errorf("initialize migrations: %w", err)
			}
		}

		var version int
		if err := tx.Model(&schemaMigration{}).Select("COALESCE(MAX(version), 0)").Scan(&version).Error; err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}

		legacyUsers := tx.Migrator().HasTable("admin_credentials") || tx.Migrator().HasTable("users") && !tx.Migrator().HasColumn(&User{}, "CredentialVersion")
		if version >= 1 && version <= 4 && legacyUsers {
			return fmt.Errorf("database schema version %d predates the release baseline; upgrade to Campfire commit 204d134 first", version)
		}

		if version == schemaVersion {
			if !tx.Migrator().HasColumn(&Server{}, "StartupOverride") {
				if err := tx.Migrator().AddColumn(&Server{}, "StartupOverride"); err != nil {
					return err
				}
			}

			if !tx.Migrator().HasColumn(&Server{}, "GameIntegration") {
				if err := tx.Migrator().AddColumn(&Server{}, "GameIntegration"); err != nil {
					return err
				}
			}

			if !tx.Migrator().HasColumn(&Server{}, "StartupCommand") {
				if err := tx.Migrator().AddColumn(&Server{}, "StartupCommand"); err != nil {
					return err
				}
			}

			for _, field := range []string{"OIDCNameManaged", "OIDCEmailManaged"} {
				if !tx.Migrator().HasColumn(&User{}, field) {
					if err := tx.Migrator().AddColumn(&User{}, field); err != nil {
						return err
					}
				}
			}

			if err := tx.AutoMigrate(&ServerResourceSample{}, &SparkResourceSample{}, &ServerResourceBucket{}, &SparkResourceBucket{}, &GameEvent{}, &GameMedia{}); err != nil {
				return err
			}

			return removeDiskHistory(tx)
		}

		prerelease := version == 5 && legacyUsers || version == 6 && tx.Migrator().HasColumn(&Schedule{}, "version")
		if version != 0 && !prerelease {
			return fmt.Errorf("database schema version %d is newer than supported version %d", version, schemaVersion)
		}

		if version != 0 {
			if err := importPrereleaseSchema(tx); err != nil {
				return fmt.Errorf("import pre-release schema: %w", err)
			}
		}

		if err := tx.AutoMigrate(
			&Spark{}, &Ember{}, &Role{}, &RolePermission{}, &User{}, &UserSession{},
			&OIDCIdentity{}, &Server{}, &ServerAccessGrant{}, &SparkAccessGrant{},
			&SparkIdentity{}, &Schedule{}, &ScheduleRun{}, &AuditEvent{},
			&ServerTransfer{}, &BackupPolicy{}, &Backup{}, &APIToken{},
			&Notification{}, &NotificationRecipient{}, &NotificationDelivery{}, &AlertState{},
			&ServerResourceSample{}, &SparkResourceSample{},
			&ServerResourceBucket{}, &SparkResourceBucket{},
			&GameEvent{},
			&GameMedia{},
		); err != nil {
			return fmt.Errorf("apply initial schema: %w", err)
		}

		if version == 0 {
			if err := seedRoles(tx); err != nil {
				return fmt.Errorf("seed built-in roles: %w", err)
			}
		}

		if tx.Name() == "sqlite" {
			var violations []struct{ Table string }
			if err := tx.Raw("PRAGMA foreign_key_check").Scan(&violations).Error; err != nil {
				return err
			}

			if len(violations) != 0 {
				return errors.New("migration produced invalid foreign keys")
			}
		}

		if err := tx.Where("1 = 1").Delete(&schemaMigration{}).Error; err != nil {
			return err
		}

		if err := tx.Create(&schemaMigration{Version: schemaVersion}).Error; err != nil {
			return fmt.Errorf("record initial migration: %w", err)
		}

		return nil
	})
}
