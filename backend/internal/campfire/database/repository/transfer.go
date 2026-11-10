package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// TransferRepository owns durable transfer history and atomic reservations.
type TransferRepository struct{ db *gorm.DB }

// Jobs returns the shared checkpoint repository.
func (r *TransferRepository) Jobs() *JobRepository { return NewJobRepository(r.db) }

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

	var jobs []database.ServerJob
	if err := unfinishedJobs(tx).Where("type = ? AND source_server_id <> ? AND (source_spark_id = ? OR target_spark_id = ?)", model.JobTransfer, excludeID, sparkID, sparkID).Find(&jobs).Error; err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(servers))
	for _, server := range servers {
		seen[server.ID] = true
	}

	for _, job := range jobs {
		if seen[job.SourceServerID] {
			continue
		}

		work, err := NewJobRepository(tx).transferWork(tx.Statement.Context, job)
		if err != nil {
			return nil, err
		}

		configuration := work.TargetConfiguration
		if job.SourceSparkID == sparkID {
			configuration = work.SourceConfiguration
		}

		servers = append(servers, transferRecord(job.SourceServerID, configuration))
		seen[job.SourceServerID] = true
	}

	return servers, nil
}

func activeTransfer(tx *gorm.DB, serverID string) error {
	if err := guardJob(tx, serverID); err != nil {
		return err
	}

	return nil
}

// Guard rejects mutations during an active transfer or copy.
func (r *TransferRepository) Guard(ctx context.Context, serverID string) error {
	return activeTransfer(r.db.WithContext(ctx), serverID)
}

// Create reserves destination capacity and freezes the validated configuration
// and definition revision atomically, including against other panel processes.
func (r *TransferRepository) Create(ctx context.Context, transfer model.TransferWork, expectedEmberHash string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if err := activeTransfer(tx, transfer.ServerID); err != nil {
			return err
		}

		server, err := NewServerRepository(tx).Get(ctx, transfer.ServerID)
		if err != nil {
			return err
		}

		if server.EmberHash != expectedEmberHash || server.EmberReviewRequired || !model.SameRuntimeConfiguration(server.ServerConfiguration, transfer.SourceConfiguration) {
			return ErrJobState
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

		data, err := json.Marshal(model.TransferJobData{SourcePorts: transfer.SourceConfiguration.Ports, TargetPorts: transfer.TargetConfiguration.Ports})
		if err != nil {
			return err
		}

		job := database.ServerJob{ID: transfer.ID, Type: model.JobTransfer, SourceServerID: transfer.ServerID, TargetServerID: transfer.ServerID, SourceSparkID: transfer.SourceSparkID, TargetSparkID: transfer.TargetSparkID, Data: string(data), Status: model.JobPending, Step: model.JobPreparing, Attempt: 1, CreatedAt: transfer.CreatedAt, UpdatedAt: transfer.UpdatedAt}

		return tx.Create(&job).Error
	})
}

// Get resolves transfer work from the protected server and placement data.
func (r *TransferRepository) Get(ctx context.Context, id string) (model.TransferWork, error) {
	store := NewJobRepository(r.db)

	job, err := store.Get(ctx, id)
	if err != nil {
		return model.TransferWork{}, err
	}

	if job.Type != model.JobTransfer {
		return model.TransferWork{}, ErrAccessNotFound
	}

	return store.transferWork(ctx, job)
}

// Advance conditionally checkpoints a transfer workflow step.
func (r *TransferRepository) Advance(ctx context.Context, id string, phase model.TransferPhase, changes map[string]any) error {
	store := NewJobRepository(r.db)

	job, err := expectedJob(ctx, store, id)
	if err != nil {
		return err
	}

	if job.Type != model.JobTransfer || transferPhase(job) != phase {
		return ErrJobState
	}

	update := map[string]any{}

	for _, field := range []string{"error", "transferred_bytes", "total_bytes", "digest"} {
		if value, ok := changes[field]; ok {
			update[field] = value
		}
	}

	if value, ok := changes["phase"]; ok {
		next := model.TransferPhase(fmt.Sprint(value))
		switch next {
		case model.TransferPhaseFailed:
			update["status"] = model.JobFailed
			if resume, ok := changes["resume_phase"]; ok {
				update["step"] = transferStep(model.TransferPhase(fmt.Sprint(resume)))
			}
		case model.TransferPhaseCancelling:
			update["status"] = model.JobCancelling
		case model.TransferPhaseComplete:
			return store.Finish(ctx, job, false)
		case model.TransferPhaseCancelled:
			return store.Finish(ctx, job, true)
		default:
			update["step"] = transferStep(next)
		}
	}

	return store.Advance(ctx, job, update)
}

// Commit atomically changes placement after verified import.
func (r *TransferRepository) Commit(ctx context.Context, id string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		store := NewJobRepository(tx)

		job, err := expectedJob(ctx, store, id)
		if err != nil {
			return err
		}

		if job.Type != model.JobTransfer || job.Status != model.JobRunning || job.Step != model.JobCommitting {
			return ErrJobState
		}

		transfer, err := store.transferWork(ctx, job)
		if err != nil {
			return err
		}

		var target database.Spark
		if err := tx.Where("id = ? AND disabled = ?", job.TargetSparkID, false).Take(&target).Error; err != nil {
			return ErrServerPlacement
		}

		servers, err := reservedServers(tx, target.ID, job.SourceServerID)
		if err != nil {
			return err
		}

		if err := checkReservations(target, append(servers, transferRecord(job.SourceServerID, transfer.TargetConfiguration))); err != nil {
			return err
		}

		ports, err := json.Marshal(transfer.TargetConfiguration.Ports)
		if err != nil {
			return err
		}

		result := tx.Model(&database.Server{}).Where("id = ? AND spark_id = ?", job.SourceServerID, job.SourceSparkID).Updates(map[string]any{"spark_id": target.ID, "ports": string(ports)})
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected != 1 {
			return ErrJobState
		}

		return store.Advance(ctx, job, map[string]any{"step": model.JobCleanup, "error": ""})
	})
}

// Retry requests a new attempt without releasing reservations.
func (r *TransferRepository) Retry(ctx context.Context, id string) error {
	return NewJobRepository(r.db).Change(ctx, id, false)
}

// Cancel requests cleanup before placement commit begins.
func (r *TransferRepository) Cancel(ctx context.Context, id string) error {
	return NewJobRepository(r.db).Change(ctx, id, true)
}
