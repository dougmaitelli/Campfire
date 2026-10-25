package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// GameMediaRepository commits attachments, deduplication, and quota checks together.
type GameMediaRepository struct{ db *gorm.DB }

// NewGameMediaRepository uses the same database and retention boundary as events.
func NewGameMediaRepository(db *gorm.DB) *GameMediaRepository { return &GameMediaRepository{db: db} }

// Append requires a retained event and its current authenticated placement.
func (r *GameMediaRepository) Append(ctx context.Context, sparkID, serverID, eventID string, media model.GameMedia, data []byte) error {
	if !media.Valid() || media.Size != int64(len(data)) || media.Size == 0 {
		return model.ErrGameMediaInvalid
	}

	digest := sha256.Sum256(data)

	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var server database.Server
		if err := tx.Select("servers.id", "servers.spark_id").Joins("JOIN sparks ON sparks.id = servers.spark_id").Where("servers.id = ? AND servers.spark_id = ? AND sparks.disabled = ? AND servers.game_integration = ?", serverID, sparkID, false, true).Take(&server).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ErrGameMediaPlacement
		} else if err != nil {
			return err
		}

		var event database.GameEvent
		if err := tx.Select("id").Where("server_id = ? AND event_id = ?", serverID, eventID).Take(&event).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ErrGameMediaEventMissing
		} else if err != nil {
			return err
		}

		var existing database.GameMedia

		err := tx.Omit("Data").Where("game_event_id = ? AND attachment_id = ?", event.ID, media.ID).Take(&existing).Error
		if err == nil {
			if existing.Digest != hex.EncodeToString(digest[:]) || existing.Metadata() != media {
				return model.ErrGameMediaConflict
			}

			return nil
		}

		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var count int64
		if err := tx.Model(&database.GameMedia{}).Where("game_event_id = ?", event.ID).Count(&count).Error; err != nil {
			return err
		}

		var usage struct {
			Total  int64
			Server int64
		}
		if err := tx.Model(&database.GameMedia{}).Select("COALESCE(SUM(size), 0) AS total, COALESCE(SUM(CASE WHEN server_id = ? THEN size ELSE 0 END), 0) AS server", serverID).Scan(&usage).Error; err != nil {
			return err
		}

		if count >= model.GameMediaEventLimit || usage.Total+media.Size > model.GameMediaTotalLimit || usage.Server+media.Size > model.GameMediaServerLimit {
			return model.ErrGameMediaLimit
		}

		row := database.GameMedia{GameEventID: event.ID, ServerID: serverID, AttachmentID: media.ID, Name: media.Name, ContentType: media.ContentType, Size: media.Size, Digest: hex.EncodeToString(digest[:]), Data: data}

		return tx.Omit("Event").Create(&row).Error
	})
}

// Get scopes downloads to both the authorized server and retained event.
func (r *GameMediaRepository) Get(ctx context.Context, serverID string, eventID int64, attachmentID string) (database.GameMedia, error) {
	var media database.GameMedia

	err := r.db.WithContext(ctx).Where("server_id = ? AND game_event_id = ? AND attachment_id = ?", serverID, eventID, attachmentID).Take(&media).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return media, model.ErrGameMediaNotFound
	}

	return media, err
}
