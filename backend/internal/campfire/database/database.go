// Package database owns SQLite and PostgreSQL setup and the release schema.
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

// Marker 2 is reserved for an equivalent schema accepted by the adoption path.
// Future schema changes must use a marker above 2 to avoid treating a different
// schema as a metadata-only adoption.
const schemaVersion = 8

// Open accepts a SQLite file path or a postgres:// / postgresql:// URL.
// It initializes the same transactional baseline schema on either backend.
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

// baselineModels is the complete schema, not a sequence of development upgrades.
func baselineModels() []any {
	return []any{
		&Spark{}, &Ember{}, &Role{}, &RolePermission{}, &User{}, &UserSession{},
		&OIDCIdentity{}, &Server{}, &ServerAccessGrant{}, &SparkAccessGrant{},
		&SparkIdentity{}, &Schedule{}, &ScheduleRun{}, &AuditEvent{}, &MonitoringAlert{},
		&ServerJob{}, &BackupPolicy{}, &Backup{}, &APIToken{},
		&Notification{}, &NotificationRecipient{}, &NotificationDelivery{}, &AlertState{},
		&ServerResourceSample{}, &SparkResourceSample{}, &ServerResourceBucket{}, &SparkResourceBucket{},
		&GameEvent{}, &GameEventFieldLabel{}, &GameIntegrationRegistration{}, &GameMedia{},
		&LogAlertIssue{}, &Integration{}, &ServerIntegration{}, &IntegrationMessage{}, &ServerCrash{},
		&StatusPage{}, &StatusPageEntry{},
	}
}

func migrate(db *gorm.DB) error {
	return Transaction(db, func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable(&schemaMigration{}) {
			tables, err := tx.Migrator().GetTables()
			if err != nil {
				return err
			}

			if len(tables) != 0 {
				return errors.New("unversioned database is not empty; older development data is unsupported")
			}

			if err := tx.Migrator().CreateTable(&schemaMigration{}); err != nil {
				return fmt.Errorf("initialize schema version: %w", err)
			}

			if err := tx.AutoMigrate(baselineModels()...); err != nil {
				return fmt.Errorf("initialize baseline schema: %w", err)
			}

			if err := seedRoles(tx); err != nil {
				return fmt.Errorf("seed built-in roles: %w", err)
			}

			return tx.Create(&schemaMigration{Version: schemaVersion}).Error
		}

		var versions []int
		if err := tx.Model(&schemaMigration{}).Pluck("version", &versions).Error; err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}

		if len(versions) != 1 || (versions[0] != schemaVersion && versions[0] != 1 && versions[0] != 2 && versions[0] != 3 && versions[0] != 4 && versions[0] != 5 && versions[0] != 6 && versions[0] != 7) {
			return fmt.Errorf("unsupported database schema history %v; expected baseline version %d", versions, schemaVersion)
		}

		if versions[0] < 5 && !tx.Migrator().HasTable(&ServerCrash{}) {
			if err := tx.Migrator().CreateTable(&ServerCrash{}); err != nil {
				return err
			}
		}

		if versions[0] < 6 {
			for _, row := range []any{&StatusPage{}, &StatusPageEntry{}} {
				if !tx.Migrator().HasTable(row) {
					if err := tx.Migrator().CreateTable(row); err != nil {
						return err
					}
				}
			}
		}

		if versions[0] < 7 {
			if err := migrateGameAddresses(tx); err != nil {
				return err
			}
		}

		if versions[0] < 8 {
			for _, row := range []any{&ServerResourceBucket{}, &SparkResourceBucket{}} {
				for _, field := range []string{"OnlineSeconds", "OfflineSeconds"} {
					if !tx.Migrator().HasColumn(row, field) {
						if err := tx.Migrator().AddColumn(row, field); err != nil {
							return err
						}
					}
				}
			}
		}

		legacy := versions[0] <= 2 && !tx.Migrator().HasColumn(&Backup{}, "id")
		if err := validateBaseline(tx, legacy, versions[0] < 4); err != nil {
			return err
		}

		if versions[0] < 4 && !tx.Migrator().HasColumn(&Server{}, "deletion_pending") {
			if err := tx.Migrator().AddColumn(&Server{}, "DeletionPending"); err != nil {
				return err
			}
		}

		if legacy {
			if err := migrateIndependentBackups(tx); err != nil {
				return err
			}
		}

		if err := validateBackupIdentity(tx); err != nil {
			return err
		}

		if versions[0] != schemaVersion {
			return tx.Model(&schemaMigration{}).Where("version = ?", versions[0]).Update("version", schemaVersion).Error
		}

		return nil
	})
}

func validateBaseline(tx *gorm.DB, legacyBackups, legacyDeletion bool) error {
	for _, model := range baselineModels() {
		statement := &gorm.Statement{DB: tx}
		if err := statement.Parse(model); err != nil {
			return err
		}

		if !tx.Migrator().HasTable(model) {
			return fmt.Errorf("database does not match the release baseline: missing table %s; older development data is unsupported", statement.Schema.Table)
		}

		columns, err := tx.Migrator().ColumnTypes(model)
		if err != nil {
			return fmt.Errorf("inspect baseline table %s: %w", statement.Schema.Table, err)
		}

		present := make(map[string]bool, len(columns))
		for _, column := range columns {
			present[column.Name()] = true
		}

		for _, field := range statement.Schema.Fields {
			if legacyDeletion && statement.Schema.Table == "servers" && field.DBName == "deletion_pending" {
				continue
			}

			if legacyBackups && statement.Schema.Table == "backups" && (field.DBName == "id" || field.DBName == "owner_user_id" || field.DBName == "local_store" || field.DBName == "local_key") {
				continue
			}

			if field.DBName != "" && !field.IgnoreMigration && !present[field.DBName] {
				return fmt.Errorf("database does not match the release baseline: missing column %s.%s; older development data is unsupported", statement.Schema.Table, field.DBName)
			}
		}
	}

	return nil
}
