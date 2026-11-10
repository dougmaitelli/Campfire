package repository

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// CopyRepository validates copy creation and resolves protected workflow inputs.
type CopyRepository struct {
	db        *gorm.DB
	publisher *EventPublisher
}

// Jobs returns the shared checkpoint repository.
func (r *CopyRepository) Jobs() *JobRepository { return NewJobRepository(r.db, r.publisher) }

// NewCopyRepository constructs the durable copy store.
func NewCopyRepository(db *gorm.DB, publishers ...*EventPublisher) *CopyRepository {
	return &CopyRepository{db: db, publisher: eventPublisher(db, publishers)}
}

// Guard serializes copies with existing placement and mutation reservations.
func (r *CopyRepository) Guard(ctx context.Context, id string) error {
	return activeTransfer(r.db.WithContext(ctx), id)
}

// Policy is available to the copy editor without exposing backup history.
func (r *CopyRepository) Policy(ctx context.Context, id string) (model.BackupPolicy, error) {
	return NewBackupRepository(r.db).Policy(ctx, id)
}

// Create validates the source snapshot and creates the server, policy, and job
// in one serialized reservation transaction. Quick suggestions use all servers.
func (r *CopyRepository) Create(ctx context.Context, source model.Server, ember model.Ember, input model.ServerConfiguration, quick, files bool, policy *model.BackupPolicy, create func(*ServerRepository, *EmberRepository, model.ServerConfiguration) (model.Server, error)) (model.Server, error) {
	var target model.Server

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if err := activeTransfer(tx, source.ID); err != nil {
			return err
		}

		store := &ServerRepository{db: tx}

		current, err := store.Get(ctx, source.ID)
		if err != nil {
			return err
		}

		if current.EmberReviewRequired || current.EmberHash != source.EmberHash || !model.SameRuntimeConfiguration(current.ServerConfiguration, source.ServerConfiguration) {
			return ErrJobState
		}

		currentEmber, err := NewEmberRepository(tx).Get(ctx, source.EmberID)
		if err != nil {
			return err
		}

		if model.EmberHash(currentEmber) != model.EmberHash(ember) {
			return ErrJobState
		}

		if quick {
			input, err = copyDefaults(tx, source.ServerConfiguration, input.SparkID)
			if err != nil {
				return err
			}
		}

		target, err = create(store, NewEmberRepository(tx), input)
		if err != nil {
			return err
		}

		selected, err := NewBackupRepository(tx).Policy(ctx, source.ID)
		if err != nil {
			return err
		}

		if policy != nil {
			selected = *policy
		}

		if err := NewBackupRepository(tx).SavePolicy(ctx, target.ID, selected); err != nil {
			return err
		}

		if !files {
			return nil
		}

		now := time.Now().Unix()
		job := database.ServerJob{ID: rand.Text(), Type: model.JobCopy, SourceServerID: source.ID, TargetServerID: target.ID, SourceSparkID: source.SparkID, TargetSparkID: target.SparkID, Data: "{}", Status: model.JobPending, Step: model.JobPreparing, Attempt: 1, CreatedAt: now, UpdatedAt: now}

		return NewJobRepository(tx, r.publisher).Create(ctx, &job)
	})

	return target, err
}

func copyDefaults(tx *gorm.DB, source model.ServerConfiguration, sparkID string) (model.ServerConfiguration, error) {
	var spark database.Spark
	if err := tx.Where("id = ? AND disabled = ?", sparkID, false).Take(&spark).Error; err != nil {
		return source, ErrServerPlacement
	}

	var names []string
	if err := tx.Model(&database.Server{}).Pluck("name", &names).Error; err != nil {
		return source, err
	}

	used := make(map[string]bool, len(names))
	for _, name := range names {
		used[strings.ToLower(name)] = true
	}

	base := []rune(source.Name)

	for number := 1; ; number++ {
		suffix := " (copy)"
		if number > 1 {
			suffix = fmt.Sprintf(" (copy %d)", number)
		}

		source.Name = string(base[:min(len(base), limits.ResourceNameLength-len(suffix))]) + suffix
		if !used[strings.ToLower(source.Name)] {
			break
		}
	}

	servers, err := reservedServers(tx, sparkID, "")
	if err != nil {
		return source, err
	}

	var reserved []model.ServerPort
	for _, server := range servers {
		reserved = append(reserved, server.Ports...)
	}

	ports := make([]model.ServerPort, 0, len(source.Ports))
	for _, mapping := range source.Ports {
		found := false

		for _, allocation := range spark.PortAllocations {
			if allocation.Protocol != mapping.Protocol {
				continue
			}

			for port := allocation.Start; port <= allocation.End; port++ {
				candidate := model.ServerPort{BindIP: allocation.BindIP, HostPort: port, ContainerPort: mapping.ContainerPort, Protocol: mapping.Protocol}
				if copyPortAvailable(candidate, reserved) {
					ports, reserved = append(ports, candidate), append(reserved, candidate)
					found = true

					break
				}
			}

			if found {
				break
			}
		}

		if !found {
			return source, serverConflictError("The destination has insufficient available ports for this copy.")
		}
	}

	source.SparkID, source.Ports = sparkID, ports

	return source, nil
}

func copyPortAvailable(candidate model.ServerPort, reserved []model.ServerPort) bool {
	ip, _ := netip.ParseAddr(candidate.BindIP)
	for _, port := range reserved {
		other, _ := netip.ParseAddr(port.BindIP)
		if port.HostPort == candidate.HostPort && port.Protocol == candidate.Protocol && (ip == other || ip.BitLen() == other.BitLen() && (ip.IsUnspecified() || other.IsUnspecified())) {
			return false
		}
	}

	return true
}

// Get resolves execution inputs from protected records.
func (r *CopyRepository) Get(ctx context.Context, id string) (model.CopyWork, error) {
	store := r.Jobs()

	job, err := store.Get(ctx, id)
	if err != nil {
		return model.CopyWork{}, err
	}

	if job.Type != model.JobCopy {
		return model.CopyWork{}, ErrAccessNotFound
	}

	return store.copyWork(ctx, job)
}

// CopyArchive is the verified source archive checkpoint.
type CopyArchive struct {
	Size   int64
	Digest string
}

// CopyCheckpoint exposes only supported workflow updates. Operation identities
// are derived from Attempt and cannot be silently ignored as legacy field names.
type CopyCheckpoint struct {
	Phase            model.CopyPhase
	Archive          *CopyArchive
	Offset           *int64
	RestartOperation bool
}

// Advance conditionally checkpoints a copy workflow step.
func (r *CopyRepository) Advance(ctx context.Context, id string, phase model.CopyPhase, checkpoint CopyCheckpoint) error {
	store := r.Jobs()

	job, err := expectedJob(ctx, store, id)
	if err != nil {
		return err
	}

	if job.Type != model.JobCopy || copyPhase(job) != phase {
		return ErrJobState
	}

	update := map[string]any{"error": ""}
	if checkpoint.Archive != nil {
		update["total_bytes"], update["digest"] = checkpoint.Archive.Size, checkpoint.Archive.Digest
	}

	if checkpoint.Offset != nil {
		update["transferred_bytes"] = *checkpoint.Offset
	}

	if checkpoint.Phase != "" {
		switch checkpoint.Phase {
		case model.CopyFailed:
			update["status"] = model.JobFailed
		case model.CopyCancelling:
			update["status"] = model.JobCancelling
		case model.CopyComplete:
			return store.Finish(ctx, job, false)
		case model.CopyCancelled:
			return store.Finish(ctx, job, true)
		case model.CopyPreparing, model.CopyBackup, model.CopyUploading, model.CopyRestoring, model.CopyCleanup:
			update["step"] = copyStep(checkpoint.Phase)
		default:
			return ErrJobState
		}
	}

	if checkpoint.RestartOperation {
		if phase != model.CopyBackup && phase != model.CopyRestoring {
			return ErrJobState
		}

		update["attempt"] = job.Attempt + 1
	}

	return store.Advance(ctx, job, update)
}

// Change requests retry or cancellation through the shared repository.
func (r *CopyRepository) Change(ctx context.Context, id string, cancel bool) error {
	return r.Jobs().Change(ctx, id, cancel)
}

// FinishCancel releases the destination only after runtime cleanup.
func (r *CopyRepository) FinishCancel(ctx context.Context, id string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		store := NewJobRepository(tx, r.publisher)

		job, err := expectedJob(ctx, store, id)
		if err != nil {
			return err
		}

		if job.Type != model.JobCopy || job.Status != model.JobCancelling {
			return ErrJobState
		}

		return NewCopyRepository(tx, r.publisher).finishCancel(ctx, job)
	})
}

func (r *CopyRepository) finishCancel(ctx context.Context, job database.ServerJob) error {
	if err := r.Jobs().Finish(ctx, job, true); err != nil {
		return err
	}

	if err := r.db.WithContext(ctx).Where("server_id = ?", job.TargetServerID).Delete(&database.BackupPolicy{}).Error; err != nil {
		return err
	}

	return NewServerRepository(r.db).Delete(ctx, job.TargetServerID)
}
