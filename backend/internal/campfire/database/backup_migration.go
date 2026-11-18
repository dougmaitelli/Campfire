package database

import (
	"crypto/rand"
	"fmt"

	"gorm.io/gorm"
)

func validateBackupIdentity(tx *gorm.DB) error {
	columns, err := tx.Migrator().ColumnTypes(&Backup{})
	if err != nil {
		return err
	}

	for _, column := range columns {
		primary, _ := column.PrimaryKey()

		nullable, known := column.Nullable()
		if column.Name() == "id" && !primary || column.Name() == "server_id" && (primary || !known || !nullable) {
			return fmt.Errorf("database backup identity requires a primary id and nullable server_id")
		}
	}

	if !tx.Migrator().HasIndex(&Backup{}, "idx_backups_server_name") {
		return fmt.Errorf("database backup catalog is missing its server/name uniqueness index")
	}

	return nil
}

type independentBackup Backup

func (independentBackup) TableName() string { return "backups_next" }

// Rebuild the old composite key transactionally on both SQLite and PostgreSQL.
// Archive bytes/object keys are untouched. Missing parents become NULL; their
// remote copies remain recoverable even when no old local locator is known.
func migrateIndependentBackups(tx *gorm.DB) error {
	if err := tx.Migrator().CreateTable(&independentBackup{}); err != nil {
		return fmt.Errorf("create independent backup catalog: %w", err)
	}

	var rows []Backup
	if err := tx.Table("backups").Find(&rows).Error; err != nil {
		return err
	}

	for _, row := range rows {
		row.ID = rand.Text()

		var servers []Server
		if err := tx.Select("id", "spark_id").Where("id = ?", row.ServerID).Find(&servers).Error; err != nil {
			return err
		}

		row.LocalKey = row.ServerID + "/" + row.Name
		if len(servers) > 0 {
			row.LocalStore = "spark:" + servers[0].SparkID
		} else {
			row.ServerID = ""
			if row.Deleting || row.LocalDeleting || row.RemoteDeleting {
				row.Error = ""
			}

			row.Deleting, row.LocalDeleting, row.RemoteDeleting = false, false, false
		}

		if err := tx.Create((*independentBackup)(&row)).Error; err != nil {
			return err
		}
	}

	if err := tx.Migrator().DropTable("backups"); err != nil {
		return err
	}

	return tx.Migrator().RenameTable("backups_next", "backups")
}
