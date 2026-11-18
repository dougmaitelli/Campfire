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
		return model.BackupPolicy{Storage: model.BackupStorageLocal}, nil
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

// CatalogBackupRow joins association and current local storage labels separately.
type CatalogBackupRow struct {
	database.Backup
	ServerName     string
	LocalSparkID   string
	LocalSparkName string
}

// ListAll returns the durable catalog without requiring online Sparks.
func (r *BackupRepository) ListAll(ctx context.Context) ([]CatalogBackupRow, error) {
	items := make([]CatalogBackupRow, 0)

	const sparkPrefix = "spark:"

	err := r.db.WithContext(ctx).Table("backups").
		Select("backups.*, COALESCE(servers.name, '') AS server_name, COALESCE(storage_spark.id, '') AS local_spark_id, COALESCE(storage_spark.name, '') AS local_spark_name").
		Joins("LEFT JOIN servers ON servers.id = backups.server_id").
		Joins("LEFT JOIN sparks AS storage_spark ON backups.local = true AND (backups.server_id IS NOT NULL OR substr(backups.local_store, 1, ?) = ?) AND storage_spark.id = CASE WHEN backups.server_id IS NOT NULL THEN servers.spark_id ELSE substr(backups.local_store, ?) END", len(sparkPrefix), sparkPrefix, len(sparkPrefix)+1).
		Order("backups.created_at DESC, backups.server_id, backups.name DESC, backups.id").Scan(&items).Error

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

// archiveCustody binds a mutation to the custody state that was authorized.
func archiveCustody(tx *gorm.DB, expected database.Backup) *gorm.DB {
	tx = tx.Model(&database.Backup{}).Where("id = ?", expected.ID)
	if expected.ServerID == "" {
		tx = tx.Where("server_id IS NULL")
	} else {
		tx = tx.Where("server_id = ?", expected.ServerID)
	}

	if expected.OwnerUserID == nil {
		return tx.Where("owner_user_id IS NULL")
	}

	return tx.Where("owner_user_id = ?", *expected.OwnerUserID)
}

// UpdateArchiveMetadata fences explicit deletion and changes of archive custody.
func (r *BackupRepository) UpdateArchiveMetadata(ctx context.Context, expected database.Backup, fields map[string]any) error {
	result := archiveCustody(r.db.WithContext(ctx), expected).Where("deleting = false AND local_deleting = false AND remote_deleting = false AND (local = true OR remote = true)").Updates(fields)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected != 1 {
		return ErrAccessConflict
	}

	return nil
}

// ClaimArchiveDeletion persists explicit deletion of an independent archive.
func (r *BackupRepository) ClaimArchiveDeletion(ctx context.Context, expected database.Backup) (bool, error) {
	result := archiveCustody(r.db.WithContext(ctx), expected).Where("server_id IS NULL AND pinned = false").Updates(map[string]any{"deleting": true, "local_deleting": true, "remote_deleting": true})
	return result.RowsAffected == 1, result.Error
}

// BackupDeletion describes the copies claimed for durable deletion.
type BackupDeletion struct {
	ArchiveID                       string
	Local, Remote, All, DisableSync bool
}

// BeginDeletion and UpdateArchiveMetadata serialize pinning against deletion claims,
// including separate panel processes sharing the same database.
func (r *BackupRepository) BeginDeletion(ctx context.Context, id, name string, deletion BackupDeletion) (bool, error) {
	query := r.db.WithContext(ctx).Model(&database.Backup{})
	if deletion.ArchiveID != "" {
		query = query.Where("id = ?", deletion.ArchiveID)
	}

	result := query.
		Where("server_id = ? AND name = ? AND pinned = false AND deleting = false AND local_deleting = false AND remote_deleting = false", id, name).
		Where("EXISTS (SELECT 1 FROM servers WHERE servers.id = backups.server_id AND servers.deletion_pending = false)").
		Updates(map[string]any{
			"deleting": deletion.All, "local_deleting": deletion.Local, "remote_deleting": deletion.Remote,
			"no_remote": gorm.Expr("no_remote OR ?", deletion.DisableSync), "error": "",
		})

	return result.RowsAffected == 1, result.Error
}

// CheckpointArchiveCopyDeletion records a confirmed removal under its original
// custody and deletion claim. An unconfirmed checkpoint keeps the claim pending.
func (r *BackupRepository) CheckpointArchiveCopyDeletion(ctx context.Context, expected database.Backup, storage model.BackupStorage) error {
	var (
		fields map[string]any
		claim  string
	)

	switch storage {
	case model.BackupStorageLocal:
		claim = "local_deleting = true"
		fields = map[string]any{"local": false, "local_deleting": false}
	case model.BackupStorageS3:
		claim = "remote_deleting = true"
		fields = map[string]any{"remote": false, "remote_deleting": false, "store_id": "", "version": ""}
	default:
		return ErrAccessInput
	}

	result := archiveCustody(r.db.WithContext(ctx), expected).Where(claim).Updates(fields)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected != 1 {
		return ErrAccessConflict
	}

	return nil
}

// Remove drops a catalog row only after physical cleanup succeeds.
func (r *BackupRepository) Remove(ctx context.Context, id, name string) error {
	return r.db.WithContext(ctx).Where("server_id = ? AND name = ?", id, name).Delete(&database.Backup{}).Error
}

// ByID resolves an archive independently of its optional server association.
func (r *BackupRepository) ByID(ctx context.Context, id string) (database.Backup, error) {
	var row database.Backup

	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrAccessNotFound
	}

	return row, err
}

// SaveArchive records a finalized standalone archive; an ID also makes retries
// idempotent without treating filenames as identities.
func (r *BackupRepository) SaveArchive(ctx context.Context, row database.Backup) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

// UpdateArchive updates storage checkpoints or metadata by immutable identity.
func (r *BackupRepository) UpdateArchive(ctx context.Context, id string, fields map[string]any) error {
	result := r.db.WithContext(ctx).Model(&database.Backup{}).Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected != 1 {
		return ErrAccessNotFound
	}

	return nil
}

// RemoveArchive forgets an archive only after all copies were explicitly removed.
func (r *BackupRepository) RemoveArchive(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&database.Backup{}).Error
}
