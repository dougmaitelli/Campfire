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
	ID            string `gorm:"primaryKey"`
	SparkID       string `gorm:"primaryKey"`
	ServerID      string
	State         model.JobState
	Data          []byte
	Size          int64
	Digest        string
	ArchiveFormat string
	ArchiveName   string
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

// Job simulates durable archive transport for the shared coordinator.
func (r *Runtime) Job(ctx context.Context, spark string, request model.JobRequest) (model.JobResult, error) {
	if !r.Connected(spark) {
		return model.JobResult{}, errors.New("mock Spark is offline")
	}

	if request.Operation == model.JobOperationWrite {
		timer := time.NewTimer(mockTransferDelay)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return model.JobResult{}, ctx.Err()
		case <-timer.C:
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if request.ID == "" || request.ServerID == "" {
		return model.JobResult{}, errors.New("missing mock transfer identity")
	}

	entry := transferRecord{ID: request.ID, SparkID: spark, ServerID: request.ServerID, State: model.JobStateAbsent}

	err := r.db.WithContext(ctx).Where("id = ? AND spark_id = ?", request.ID, spark).Take(&entry).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.JobResult{}, err
	}

	if entry.ServerID != request.ServerID {
		return model.JobResult{}, errors.New("mock transfer belongs to another server")
	}

	result, err := r.transferOperation(ctx, &entry, request)
	if err != nil {
		return model.JobResult{}, err
	}

	if err := r.db.WithContext(ctx).Save(&entry).Error; err != nil {
		return model.JobResult{}, err
	}

	result.ID, result.ServerID, result.State = entry.ID, entry.ServerID, entry.State
	result.Size, result.Digest = entry.Size, entry.Digest

	if request.Operation != model.JobOperationRead {
		result.Offset = int64(len(entry.Data))
	}

	return result, nil
}

func (r *Runtime) transferOperation(ctx context.Context, entry *transferRecord, request model.JobRequest) (model.JobResult, error) {
	result := model.JobResult{}

	switch request.Operation {
	case model.JobOperationStatus:
	case model.JobOperationExport:
		if entry.State != model.JobStateAbsent {
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
		entry.State = model.JobStateReady
	case model.JobOperationPrepare:
		if entry.State == model.JobStateAbsent || entry.State == model.JobStateCorrupt {
			entry.ArchiveFormat, entry.ArchiveName = request.ArchiveFormat, request.ArchiveName
			if request.Size <= 0 || request.Digest == "" {
				return result, errors.New("missing mock transfer archive metadata")
			}

			entry.Data, entry.Size, entry.Digest = nil, request.Size, request.Digest
			entry.State = model.JobStateReceiving
		}

		if entry.Size != request.Size || entry.Digest != request.Digest {
			return result, errors.New("mock transfer archive metadata changed")
		}
	case model.JobOperationRead:
		if entry.State != model.JobStateReady || request.Offset < 0 || request.Offset > int64(len(entry.Data)) {
			return result, errors.New("invalid mock transfer read")
		}

		end := min(request.Offset+max(1, entry.Size/mockTransferChunks), entry.Size)
		result.Data = bytes.Clone(entry.Data[request.Offset:end])
		result.Offset = end
	case model.JobOperationWrite:
		if entry.State != model.JobStateReceiving || request.Offset < 0 || request.Offset > int64(len(entry.Data)) || request.Offset+int64(len(request.Data)) > entry.Size {
			return result, errors.New("invalid mock transfer write")
		}

		if request.Offset == int64(len(entry.Data)) {
			entry.Data = append(entry.Data, request.Data...)
		} else if end := request.Offset + int64(len(request.Data)); end > int64(len(entry.Data)) || !bytes.Equal(entry.Data[request.Offset:end], request.Data) {
			return result, errors.New("conflicting mock transfer chunk")
		}
	case model.JobOperationImport:
		if entry.State == model.JobStateImported || entry.State == model.JobStatePublished || entry.State == model.JobStateActive {
			break
		}

		if entry.State != model.JobStateReceiving || int64(len(entry.Data)) != entry.Size {
			return result, errors.New("mock transfer archive is incomplete")
		}

		digest := sha256.Sum256(entry.Data)
		entry.State = model.JobStateImported

		if hex.EncodeToString(digest[:]) != entry.Digest {
			entry.State = model.JobStateCorrupt
		}

		if entry.State == model.JobStateImported && entry.ArchiveFormat == "backup" {
			if r.backupUploads == nil {
				r.backupUploads = make(map[string]*mockBackupUpload)
			}

			upload := &mockBackupUpload{server: entry.ServerID, path: entry.ArchiveName, size: entry.Size, expires: time.Now().Add(time.Hour)}
			_, _ = upload.data.Write(entry.Data)
			r.backupUploads[entry.ID] = upload

			_, err := r.uploadBackup(ctx, runtimeRecord{ServerID: entry.ServerID}, model.FileRequest{Operation: model.FileOperationBackupUploadCommit, Path: entry.ArchiveName, UploadID: entry.ID})
			if err != nil {
				return result, err
			}

			entry.State = model.JobStatePublished
		}
	case model.JobOperationActivate:
		if entry.State == model.JobStatePublished {
			entry.State = model.JobStateActive
			entry.Data = nil

			break
		}

		if entry.State == model.JobStateActive {
			break
		}

		if entry.State != model.JobStateImported && entry.State != model.JobStatePublished {
			return result, errors.New("mock transfer has not been imported")
		}

		if err := r.activateTransfer(ctx, entry); err != nil {
			return result, err
		}

		entry.State = model.JobStateActive
		entry.Data = nil
	case model.JobOperationRelease, model.JobOperationDiscard:
		if entry.State == model.JobStateActive || entry.State == model.JobStateRetired {
			return result, errors.New("mock transfer already committed")
		}

		entry.Data = nil
		entry.State = model.JobStateReleased

		if request.Operation == model.JobOperationDiscard {
			entry.State = model.JobStateDiscarded
		}

		if err := r.db.WithContext(ctx).Where("server_id = ?", entry.ServerID).Delete(&transferDeployment{}).Error; err != nil {
			return result, err
		}
	case model.JobOperationCleanup:
		entry.State, entry.Data = model.JobStateRetired, nil
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

		entry.State, entry.Data = model.JobStateActive, nil
		if err := tx.Save(entry).Error; err != nil {
			return err
		}

		return tx.Delete(&staged).Error
	})
}
