//go:build !production

package devmock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

const (
	mockTransferChunks = 24
	mockTransferDelay  = 250 * time.Millisecond
)

// Destination configuration remains separate until the real coordinator commits placement.
type transferDeployment struct {
	ServerID   string           `gorm:"primaryKey"`
	SparkID    string           `gorm:"primaryKey"`
	Deployment model.Deployment `gorm:"serializer:json"`
}

type transferRecord struct {
	ID       string `gorm:"primaryKey"`
	SparkID  string `gorm:"primaryKey"`
	ServerID string
	State    model.TransferState
	Data     []byte
	Size     int64
	Digest   string
}

func (transferRecord) TableName() string { return "mock_transfers" }

type transferPayload struct {
	Runtime runtimeRecord
	Backups []backupRecord
}

func (r *Runtime) stageDeployment(ctx context.Context, spark string, deployment model.Deployment) (model.ConfigurationReceipt, error) {
	data, err := json.Marshal(deployment)
	if err != nil {
		return model.ConfigurationReceipt{}, err
	}

	entry := transferDeployment{ServerID: deployment.ServerID, SparkID: spark, Deployment: deployment}
	if err := r.db.WithContext(ctx).Save(&entry).Error; err != nil {
		return model.ConfigurationReceipt{}, err
	}

	digest := sha256.Sum256(data)

	return model.ConfigurationReceipt{ServerID: deployment.ServerID, Revision: hex.EncodeToString(digest[:])}, nil
}

// CheckTransfer uses the same offline fixture as the other mock transports.
func (r *Runtime) CheckTransfer(spark string) error {
	if !r.Connected(spark) {
		return errors.New("mock Spark is offline")
	}

	return nil
}

// Transfer simulates Spark archive transport while the real service owns workflow and reservations.
func (r *Runtime) Transfer(ctx context.Context, spark string, request model.TransferRequest) (model.TransferResult, error) {
	if err := r.CheckTransfer(spark); err != nil {
		return model.TransferResult{}, err
	}

	if request.Operation == model.TransferOperationWrite {
		timer := time.NewTimer(mockTransferDelay)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return model.TransferResult{}, ctx.Err()
		case <-timer.C:
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if request.ID == "" || request.ServerID == "" {
		return model.TransferResult{}, errors.New("missing mock transfer identity")
	}

	entry := transferRecord{ID: request.ID, SparkID: spark, ServerID: request.ServerID, State: model.TransferStateAbsent}

	err := r.db.WithContext(ctx).Where("id = ? AND spark_id = ?", request.ID, spark).Take(&entry).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.TransferResult{}, err
	}

	if entry.ServerID != request.ServerID {
		return model.TransferResult{}, errors.New("mock transfer belongs to another server")
	}

	result, err := r.transferOperation(ctx, &entry, request)
	if err != nil {
		return model.TransferResult{}, err
	}

	if err := r.db.WithContext(ctx).Save(&entry).Error; err != nil {
		return model.TransferResult{}, err
	}

	result.ID, result.ServerID, result.State = entry.ID, entry.ServerID, entry.State
	result.Size, result.Digest = entry.Size, entry.Digest

	if request.Operation != model.TransferOperationRead {
		result.Offset = int64(len(entry.Data))
	}

	return result, nil
}

func (r *Runtime) transferOperation(ctx context.Context, entry *transferRecord, request model.TransferRequest) (model.TransferResult, error) {
	result := model.TransferResult{}

	switch request.Operation {
	case model.TransferOperationStatus:
	case model.TransferOperationExport:
		if entry.State != model.TransferStateAbsent {
			break
		}

		record, err := r.load(ctx, entry.SparkID, entry.ServerID)
		if err != nil {
			return result, err
		}

		if record.TransitionAt != 0 || (record.State != model.RuntimeStateStopped && record.State != model.RuntimeStateNotInstalled) {
			return result, errors.New("stop the mock server before transferring it")
		}

		payload := transferPayload{Runtime: record}
		if err := r.db.WithContext(ctx).Where("server_id = ?", entry.ServerID).Find(&payload.Backups).Error; err != nil {
			return result, err
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return result, err
		}

		digest := sha256.Sum256(data)
		entry.Data, entry.Size, entry.Digest = data, int64(len(data)), hex.EncodeToString(digest[:])
		entry.State = model.TransferStateReady
	case model.TransferOperationPrepare:
		if entry.State == model.TransferStateAbsent || entry.State == model.TransferStateCorrupt {
			if request.Size <= 0 || request.Digest == "" {
				return result, errors.New("missing mock transfer archive metadata")
			}

			entry.Data, entry.Size, entry.Digest = nil, request.Size, request.Digest
			entry.State = model.TransferStateReceiving
		}

		if entry.Size != request.Size || entry.Digest != request.Digest {
			return result, errors.New("mock transfer archive metadata changed")
		}
	case model.TransferOperationRead:
		if entry.State != model.TransferStateReady || request.Offset < 0 || request.Offset > int64(len(entry.Data)) {
			return result, errors.New("invalid mock transfer read")
		}

		end := min(request.Offset+max(1, entry.Size/mockTransferChunks), entry.Size)
		result.Data = bytes.Clone(entry.Data[request.Offset:end])
		result.Offset = end
	case model.TransferOperationWrite:
		if entry.State != model.TransferStateReceiving || request.Offset < 0 || request.Offset > int64(len(entry.Data)) || request.Offset+int64(len(request.Data)) > entry.Size {
			return result, errors.New("invalid mock transfer write")
		}

		if request.Offset == int64(len(entry.Data)) {
			entry.Data = append(entry.Data, request.Data...)
		} else if end := request.Offset + int64(len(request.Data)); end > int64(len(entry.Data)) || !bytes.Equal(entry.Data[request.Offset:end], request.Data) {
			return result, errors.New("conflicting mock transfer chunk")
		}
	case model.TransferOperationImport:
		if entry.State == model.TransferStateImported || entry.State == model.TransferStateActive {
			break
		}

		if entry.State != model.TransferStateReceiving || int64(len(entry.Data)) != entry.Size {
			return result, errors.New("mock transfer archive is incomplete")
		}

		digest := sha256.Sum256(entry.Data)
		entry.State = model.TransferStateImported

		if hex.EncodeToString(digest[:]) != entry.Digest {
			entry.State = model.TransferStateCorrupt
		}
	case model.TransferOperationActivate:
		if entry.State == model.TransferStateActive {
			break
		}

		if entry.State != model.TransferStateImported {
			return result, errors.New("mock transfer has not been imported")
		}

		if err := r.activateTransfer(ctx, entry); err != nil {
			return result, err
		}

		entry.State = model.TransferStateActive
		entry.Data = nil
	case model.TransferOperationRelease, model.TransferOperationDiscard:
		if entry.State == model.TransferStateActive || entry.State == model.TransferStateRetired {
			return result, errors.New("mock transfer already committed")
		}

		entry.Data = nil
		entry.State = model.TransferStateReleased

		if request.Operation == model.TransferOperationDiscard {
			entry.State = model.TransferStateDiscarded
		}

		if err := r.db.WithContext(ctx).Where("server_id = ?", entry.ServerID).Delete(&transferDeployment{}).Error; err != nil {
			return result, err
		}
	case model.TransferOperationCleanup:
		entry.State, entry.Data = model.TransferStateRetired, nil
	default:
		return result, fmt.Errorf("unsupported mock transfer operation %q", request.Operation)
	}

	return result, nil
}

func (r *Runtime) activateTransfer(ctx context.Context, entry *transferRecord) error {
	var payload transferPayload
	if err := json.Unmarshal(entry.Data, &payload); err != nil {
		return err
	}

	if payload.Runtime.ServerID != entry.ServerID {
		return errors.New("mock transfer archive belongs to another server")
	}

	var staged transferDeployment
	if err := r.db.WithContext(ctx).Where("server_id = ? AND spark_id = ?", entry.ServerID, entry.SparkID).Take(&staged).Error; err != nil {
		return err
	}

	payload.Runtime.SparkID, payload.Runtime.Deployment = entry.SparkID, staged.Deployment
	payload.Runtime.Logs += timestampLog("[Mock Spark] Server transfer completed.\n")

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&payload.Runtime).Error; err != nil {
			return err
		}

		for _, backup := range payload.Backups {
			if err := tx.Save(&backup).Error; err != nil {
				return err
			}
		}

		entry.State, entry.Data = model.TransferStateActive, nil
		if err := tx.Save(entry).Error; err != nil {
			return err
		}

		return tx.Delete(&staged).Error
	})
}
