package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// ErrTransferActive prevents configuration and placement changes while both
// Sparks hold a frozen copy. ErrTransferState requires an explicit retry/cancel.
var (
	ErrTransferActive = errors.New("server is locked for a Spark transfer")
	ErrTransferState  = errors.New("transfer state does not permit this operation")
)

// TransferRepository owns durable transfer history and atomic reservations.
type TransferRepository struct{ db *gorm.DB }

// NewTransferRepository creates a transfer repository.
func NewTransferRepository(db *gorm.DB) *TransferRepository { return &TransferRepository{db: db} }

func transferRecord(id string, c model.ServerConfiguration) database.Server {
	return database.Server{ID: id, SparkID: c.SparkID, CPULimit: c.CPULimit, MemoryMB: c.MemoryMB, DiskMB: c.DiskMB, Ports: c.Ports}
}

func reservedServers(tx *gorm.DB, sparkID, excludeID string) ([]database.Server, error) {
	var servers []database.Server
	if err := tx.Where("spark_id = ? AND id <> ?", sparkID, excludeID).Find(&servers).Error; err != nil {
		return nil, err
	}

	var transfers []database.ServerTransfer
	if err := tx.Where("active = ? AND server_id <> ? AND (source_spark_id = ? OR target_spark_id = ?)", true, excludeID, sparkID, sparkID).Find(&transfers).Error; err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(servers))
	for _, server := range servers {
		seen[server.ID] = true
	}

	for _, transfer := range transfers {
		if seen[transfer.ServerID] {
			continue
		}

		configuration := transfer.TargetConfiguration
		if transfer.SourceSparkID == sparkID {
			configuration = transfer.SourceConfiguration
		}

		servers = append(servers, transferRecord(transfer.ServerID, configuration))
		seen[transfer.ServerID] = true
	}

	return servers, nil
}

func activeTransfer(tx *gorm.DB, serverID string) error {
	var count int64
	if err := tx.Model(&database.ServerTransfer{}).Where("server_id = ? AND active = ?", serverID, true).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return ErrTransferActive
	}

	return nil
}

// Guard rejects mutations during an active transfer.
func (r *TransferRepository) Guard(ctx context.Context, serverID string) error {
	return activeTransfer(r.db.WithContext(ctx), serverID)
}

// Create reserves destination capacity and freezes placement atomically.
func (r *TransferRepository) Create(ctx context.Context, transfer database.ServerTransfer) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := activeTransfer(tx, transfer.ServerID); err != nil {
			return err
		}

		var server database.Server
		if err := tx.Where("id = ? AND spark_id = ?", transfer.ServerID, transfer.SourceSparkID).Take(&server).Error; err != nil {
			return ErrTransferState
		}

		var target database.Spark
		if err := tx.Where("id = ? AND disabled = ?", transfer.TargetSparkID, false).Take(&target).Error; err != nil {
			return ErrServerPlacement
		}

		servers, err := reservedServers(tx, target.ID, transfer.ServerID)
		if err != nil {
			return err
		}

		if err := checkReservations(target, append(servers, transferRecord(transfer.ServerID, transfer.TargetConfiguration))); err != nil {
			return err
		}

		return tx.Create(&transfer).Error
	})
}

// Get loads a transfer and its available cancellation action.
func (r *TransferRepository) Get(ctx context.Context, id string) (database.ServerTransfer, error) {
	var transfer database.ServerTransfer

	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&transfer).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrAccessNotFound
	}

	transfer.CanCancel = canCancelTransfer(transfer)

	return transfer, err
}

// List returns server history or active coordinator work.
func (r *TransferRepository) List(ctx context.Context, serverID string, active bool) ([]database.ServerTransfer, error) {
	query := r.db.WithContext(ctx).Limit(100)
	if serverID != "" {
		query = query.Where("server_id = ?", serverID)
	}

	if active {
		query = query.Where("active = ? AND phase <> ?", true, "failed").Order("created_at")
	} else {
		query = query.Order("created_at DESC")
	}

	transfers := make([]database.ServerTransfer, 0)

	err := query.Find(&transfers).Error
	for i := range transfers {
		transfers[i].CanCancel = canCancelTransfer(transfers[i])
	}

	return transfers, err
}

func canCancelTransfer(transfer database.ServerTransfer) bool {
	phase := transfer.Phase
	if phase == "failed" {
		phase = transfer.ResumePhase
	}

	return transfer.Active && phase != "committing" && phase != "cleanup" && phase != "cancelling"
}

// Advance persists progress only while the expected phase remains current.
func (r *TransferRepository) Advance(ctx context.Context, id, phase string, changes map[string]any) error {
	changes["updated_at"] = time.Now().Unix()

	result := r.db.WithContext(ctx).Model(&database.ServerTransfer{}).Where("id = ? AND active = ? AND phase = ?", id, true, phase).Updates(changes)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected != 1 {
		return ErrTransferState
	}

	return nil
}

// Commit changes placement after a verified destination import.
func (r *TransferRepository) Commit(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var transfer database.ServerTransfer
		if err := tx.Where("id = ? AND active = ? AND phase = ?", id, true, "committing").Take(&transfer).Error; err != nil {
			return ErrTransferState
		}

		var target database.Spark
		if err := tx.Where("id = ? AND disabled = ?", transfer.TargetSparkID, false).Take(&target).Error; err != nil {
			return ErrServerPlacement
		}

		servers, err := reservedServers(tx, target.ID, transfer.ServerID)
		if err != nil {
			return err
		}

		if err := checkReservations(target, append(servers, transferRecord(transfer.ServerID, transfer.TargetConfiguration))); err != nil {
			return err
		}

		ports, err := json.Marshal(transfer.TargetConfiguration.Ports)
		if err != nil {
			return err
		}

		result := tx.Model(&database.Server{}).Where("id = ? AND spark_id = ?", transfer.ServerID, transfer.SourceSparkID).Updates(map[string]any{"spark_id": target.ID, "ports": string(ports)})
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected != 1 {
			return ErrTransferState
		}

		return tx.Model(&transfer).Updates(map[string]any{"phase": "cleanup", "error": "", "updated_at": time.Now().Unix()}).Error
	})
}

// Retry resumes a failed transfer from its durable checkpoint.
func (r *TransferRepository) Retry(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		transfer, err := (&TransferRepository{db: tx}).Get(ctx, id)
		if err != nil {
			return err
		}

		if !transfer.Active || transfer.Phase != "failed" || transfer.ResumePhase == "" {
			return ErrTransferState
		}

		return tx.Model(&transfer).Updates(map[string]any{"phase": transfer.ResumePhase, "error": "", "updated_at": time.Now().Unix()}).Error
	})
}

// Cancel requests cleanup before placement has changed.
func (r *TransferRepository) Cancel(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		transfer, err := (&TransferRepository{db: tx}).Get(ctx, id)
		if err != nil {
			return err
		}

		phase := transfer.Phase
		if phase == "failed" {
			phase = transfer.ResumePhase
		}

		if !transfer.Active || phase == "committing" || phase == "cleanup" {
			return ErrTransferState
		}

		cancelFrom := transfer.CancelFrom
		if phase != "cancelling" {
			cancelFrom = phase
		}

		return tx.Model(&transfer).Updates(map[string]any{"phase": "cancelling", "cancel_from": cancelFrom, "error": "", "updated_at": time.Now().Unix()}).Error
	})
}
