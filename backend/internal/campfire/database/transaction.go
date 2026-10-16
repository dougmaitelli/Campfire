package database

import "gorm.io/gorm"

func lockTransaction(tx *gorm.DB) error {
	return tx.Session(&gorm.Session{NewDB: true}).Exec("SELECT pg_advisory_xact_lock(1128353094)").Error
}

// Standalone writes use GORM's default transaction and must obey the same lock
// as multi-step repository operations (for example revocation versus login).
func registerWriteLocks(db *gorm.DB) error {
	lock := func(tx *gorm.DB) {
		if tx.Error == nil {
			_ = tx.AddError(lockTransaction(tx))
		}
	}

	if err := db.Callback().Create().Before("gorm:create").Register("campfire:serialize", lock); err != nil {
		return err
	}

	if err := db.Callback().Update().Before("gorm:update").Register("campfire:serialize", lock); err != nil {
		return err
	}

	return db.Callback().Delete().Before("gorm:delete").Register("campfire:serialize", lock)
}

// Transaction preserves the serialized read/check/write semantics of SQLite's
// immediate transactions when using PostgreSQL. The transaction-scoped advisory
// lock is released on commit, rollback, or connection loss.
func Transaction(db *gorm.DB, fn func(*gorm.DB) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if tx.Name() == "postgres" {
			if err := lockTransaction(tx); err != nil {
				return err
			}
		}

		return fn(tx)
	})
}
