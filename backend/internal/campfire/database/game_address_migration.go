package database

import "gorm.io/gorm"

// migrateGameAddresses adds optional server overrides. Existing Spark defaults
// remain inherited, rather than being copied into fixed per-server values.
func migrateGameAddresses(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&Server{}) {
		return nil
	}

	if !tx.Migrator().HasColumn(&Server{}, "game_address") {
		return tx.Migrator().AddColumn(&Server{}, "GameAddress")
	}

	return nil
}
