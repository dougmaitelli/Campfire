package repository

import (
	"context"
	"errors"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BackupRepository persists catalog entries, deletion intents and retention settings.
type BackupRepository struct{ db *gorm.DB }

// NewBackupRepository creates the backup catalog.
func NewBackupRepository(db *gorm.DB) *BackupRepository { return &BackupRepository{db: db} }

// Policy returns local storage with unlimited retention unless explicitly configured.
func (r *BackupRepository) Policy(ctx context.Context, id string) (model.BackupPolicy, error) {
	var row database.BackupPolicy

	err := r.db.WithContext(ctx).Where("server_id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.BackupPolicy{Storage: "local"}, nil
	}

	return model.BackupPolicy{Storage: row.Storage, LocalKeepLast: row.LocalKeepLast, LocalMaxAgeDays: row.LocalMaxAgeDays, RemoteKeepLast: row.RemoteKeepLast, RemoteMaxAgeDays: row.RemoteMaxAgeDays, Savedata: model.BackupRetention{LocalKeepLast: row.SavedataLocalKeepLast, LocalMaxAgeDays: row.SavedataLocalMaxAgeDays, RemoteKeepLast: row.SavedataRemoteKeepLast, RemoteMaxAgeDays: row.SavedataRemoteMaxAgeDays}}, err
}

// SavePolicy updates retention under the same placement guard as transfers.
func (r *BackupRepository) SavePolicy(ctx context.Context, id string, p model.BackupPolicy) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if err := activeTransfer(tx, id); err != nil {
			return err
		}

		var server database.Server
		if err := tx.Where("id = ?", id).Take(&server).Error; err != nil {
			return ErrServerNotFound
		}

		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&database.BackupPolicy{ServerID: id, Storage: p.Storage, LocalKeepLast: p.LocalKeepLast, LocalMaxAgeDays: p.LocalMaxAgeDays, RemoteKeepLast: p.RemoteKeepLast, RemoteMaxAgeDays: p.RemoteMaxAgeDays, SavedataLocalKeepLast: p.Savedata.LocalKeepLast, SavedataLocalMaxAgeDays: p.Savedata.LocalMaxAgeDays, SavedataRemoteKeepLast: p.Savedata.RemoteKeepLast, SavedataRemoteMaxAgeDays: p.Savedata.RemoteMaxAgeDays}).Error
	})
}

// Discover adds a completed local archive without resetting remote or deletion state.
func (r *BackupRepository) Discover(ctx context.Context, b database.Backup) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&b).Error
}

// List returns all catalog entries for a server, newest first.
func (r *BackupRepository) List(ctx context.Context, id string) ([]database.Backup, error) {
	items := make([]database.Backup, 0)
	err := r.db.WithContext(ctx).Where("server_id = ?", id).Order("created_at DESC, name DESC").Find(&items).Error

	return items, err
}

// CatalogBackupRow joins a catalog record to its server name.
type CatalogBackupRow struct {
	database.Backup
	ServerName string
}

// ListAll returns the durable catalog without requiring online Sparks.
func (r *BackupRepository) ListAll(ctx context.Context) ([]CatalogBackupRow, error) {
	items := make([]CatalogBackupRow, 0)
	err := r.db.WithContext(ctx).Table("backups").Select("backups.*, servers.name AS server_name").Joins("JOIN servers ON servers.id = backups.server_id").Order("backups.created_at DESC, backups.server_id, backups.name DESC").Scan(&items).Error

	return items, err
}

// Get resolves a single immutable backup identity.
func (r *BackupRepository) Get(ctx context.Context, id, name string) (database.Backup, error) {
	var item database.Backup

	err := r.db.WithContext(ctx).Where("server_id = ? AND name = ?", id, name).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrAccessNotFound
	}

	return item, err
}

// Update saves upload checkpoints or deletion intent without recreating a removed row.
func (r *BackupRepository) Update(ctx context.Context, id, name string, fields map[string]any) error {
	result := r.db.WithContext(ctx).Model(&database.Backup{}).Where("server_id = ? AND name = ?", id, name).Updates(fields)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected != 1 {
		return ErrAccessNotFound
	}

	return nil
}

// Remove drops a catalog row only after physical cleanup succeeds.
func (r *BackupRepository) Remove(ctx context.Context, id, name string) error {
	return r.db.WithContext(ctx).Where("server_id = ? AND name = ?", id, name).Delete(&database.Backup{}).Error
}

// ForgetServer removes its empty policy after all catalog entries are cleaned.
func (r *BackupRepository) ForgetServer(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("server_id = ?", id).Delete(&database.BackupPolicy{}).Error
}
