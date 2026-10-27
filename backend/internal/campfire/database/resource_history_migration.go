package database

import "gorm.io/gorm"

// Current disk readings remain in collector snapshots; only CPU and memory roll up.
func removeDiskHistory(tx *gorm.DB) error {
	for _, table := range []string{"server_resource_buckets", "spark_resource_buckets"} {
		for _, column := range []string{"disk_count", "disk_sum", "disk_peak", "disk_limit_sum", "disk_unlimited"} {
			if tx.Migrator().HasColumn(table, column) {
				if err := tx.Exec("ALTER TABLE " + table + " DROP COLUMN " + column).Error; err != nil {
					return err
				}
			}
		}
	}

	return nil
}
