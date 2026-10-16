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

	"campfire/internal/campfire/recurrence"

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

		if version > 4 {
			return fmt.Errorf("database schema version %d is newer than supported version 4", version)
		}

		if version == 4 {
			return nil
		}

		if version == 3 && tx.Migrator().HasColumn(&Schedule{}, "OnlyWhenRunning") && tx.Migrator().HasColumn(&ScheduleRun{}, "OnlyWhenRunning") && !tx.Migrator().HasColumn(&Schedule{}, "interval_minutes") {
			return migrateUserEmail(tx)
		}

		if version == 1 {
			for _, field := range []string{"Name", "Picture"} {
				if err := tx.Migrator().AddColumn(&User{}, field); err != nil {
					return fmt.Errorf("add user profile field %s: %w", field, err)
				}
			}

			if err := tx.Create(&schemaMigration{Version: 2}).Error; err != nil {
				return err
			}

			version = 2
		}

		if version >= 2 {
			for _, field := range []string{"CronExpression", "TimeZone", "OnlyWhenRunning"} {
				if !tx.Migrator().HasColumn(&Schedule{}, field) {
					if err := tx.Migrator().AddColumn(&Schedule{}, field); err != nil {
						return fmt.Errorf("add schedule field %s: %w", field, err)
					}
				}
			}

			if !tx.Migrator().HasColumn(&ScheduleRun{}, "OnlyWhenRunning") {
				if err := tx.Migrator().AddColumn(&ScheduleRun{}, "OnlyWhenRunning"); err != nil {
					return fmt.Errorf("add execution running condition: %w", err)
				}
			}

			if tx.Migrator().HasColumn(&Schedule{}, "interval_minutes") {
				if err := tx.Exec("ALTER TABLE schedules DROP COLUMN interval_minutes").Error; err != nil {
					return err
				}
			}

			next, err := recurrence.Next("0 3 * * *", "UTC", time.Now(), 1)
			if err != nil {
				return err
			}

			query := tx.Model(&Schedule{}).Where("1 = 1")
			if version == 3 {
				query = tx.Model(&Schedule{}).Where("cron_expression = ''")
			}

			if err := query.Updates(map[string]any{"cron_expression": "0 3 * * *", "time_zone": "UTC", "next_run_at": next[0]}).Error; err != nil {
				return err
			}

			if version == 3 {
				return migrateUserEmail(tx)
			}

			if err := tx.Create(&schemaMigration{Version: 3}).Error; err != nil {
				return err
			}

			return migrateUserEmail(tx)
		}

		if err := tx.AutoMigrate(
			&Spark{}, &Ember{}, &Role{}, &RolePermission{}, &User{}, &UserSession{},
			&OIDCIdentity{}, &Server{}, &ServerOwnership{}, &ServerMember{}, &SparkRole{},
			&SparkEnrollment{}, &SparkIdentity{}, &Schedule{}, &ScheduleRun{}, &AuditEvent{},
			&ServerTransfer{}, &BackupPolicy{}, &Backup{}, &APIToken{},
			&Notification{}, &NotificationRecipient{}, &NotificationDelivery{}, &AlertState{},
		); err != nil {
			return fmt.Errorf("apply initial schema: %w", err)
		}

		if err := seedRoles(tx); err != nil {
			return fmt.Errorf("seed built-in roles: %w", err)
		}

		if err := tx.Create(&schemaMigration{Version: 4}).Error; err != nil {
			return fmt.Errorf("record initial migration: %w", err)
		}

		return nil
	})
}

func migrateUserEmail(tx *gorm.DB) error {
	if !tx.Migrator().HasColumn(&User{}, "Email") {
		if err := tx.Migrator().AddColumn(&User{}, "Email"); err != nil {
			return fmt.Errorf("add user email: %w", err)
		}
	}

	return tx.Create(&schemaMigration{Version: 4}).Error
}
