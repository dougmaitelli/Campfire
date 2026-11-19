package fileops

import (
	"context"
	"os"
	"slices"
	"sync"
	"time"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
	"campfire/internal/shared/workprogress"
)

// Membership is protected by Manager.mu; snapshots by mu. Workers never acquire
// Manager.mu while holding mu. Terminal results retain their idempotency key for
// limits.ArchiveResultRetention, bounded to 128 operations. RPC cancellation never owns work.
type compressionSession struct {
	mu       sync.Mutex
	request  model.FileRequest
	status   model.CompressionStatus
	cancel   context.CancelFunc
	finished time.Time
}

// compressionActive includes cancellation drainage, not just new writes.
func (m *Manager) compressionActive(serverID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.compressionActiveLocked(serverID)
}

func (m *Manager) compressionActiveLocked(serverID string) bool {
	for _, s := range m.compressions {
		s.mu.Lock()
		active := s.request.ServerID == serverID && s.finished.IsZero()
		s.mu.Unlock()

		if active {
			return true
		}
	}

	return false
}

func (m *Manager) compression(ctx context.Context, d model.Deployment, volume string, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: q.ServerID, Path: q.Destination}
	if q.Compression == nil || len(q.Compression.ID) < 32 || !utils.IsValidResourceID(q.Compression.ID) || q.CompressionOwner == "" {
		return result, Failure(model.FileInvalid, "A valid compression operation and owner are required.")
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return result, Failure(model.FileBusy, "File operations are shutting down.")
	}

	for id, s := range m.compressions {
		s.mu.Lock()
		expired := !s.finished.IsZero() && time.Since(s.finished) >= limits.ArchiveResultRetention
		s.mu.Unlock()

		if expired {
			delete(m.compressions, id)
		}
	}

	s := m.compressions[q.Compression.ID]
	if s != nil && (s.request.ServerID != q.ServerID || s.request.CompressionOwner != q.CompressionOwner || s.request.Path != q.Path || s.request.Destination != q.Destination || !slices.Equal(s.request.Paths, q.Paths)) {
		m.mu.Unlock()
		return result, Failure(model.FileNotFound, "Compression operation not found.")
	}

	if s == nil {
		if q.Operation != model.FileOperationCompressStart {
			m.mu.Unlock()
			return result, Failure(model.FileNotFound, "Compression result is unavailable. Check the destination before starting again.")
		}

		if len(m.compressions) >= 128 || m.compressionActiveLocked(q.ServerID) {
			m.mu.Unlock()
			return result, Failure(model.FileBusy, "Wait for active archive operations or retained results to expire.")
		}

		for _, other := range m.operations {
			other.mu.Lock()
			active := other.serverID == q.ServerID && other.busy()
			other.mu.Unlock()

			if active {
				m.mu.Unlock()
				return result, Failure(model.FileBusy, "Wait for extraction to finish.")
			}
		}

		if !ValidPath(q.Destination, false) || len(q.Paths) > 1000 {
			m.mu.Unlock()
			return result, Failure(model.FileInvalid, "Choose a safe archive destination and at most 1,000 items.")
		}

		operationContext, cancel := context.WithTimeout(m.ctx, 30*time.Minute)
		// Retain only archive identity, never unrelated editor/upload payloads.
		request := model.FileRequest{ServerID: q.ServerID, Path: q.Path, Paths: slices.Clone(q.Paths), Destination: q.Destination, CompressionOwner: q.CompressionOwner}
		s = &compressionSession{request: request, cancel: cancel, status: model.CompressionStatus{ID: q.Compression.ID, State: model.CompressionRunning}}
		m.compressions[q.Compression.ID] = s

		m.wg.Add(1)
		go func() { defer m.wg.Done(); m.compress(operationContext, d, volume, s) }()
	}
	m.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()

	if q.Operation == model.FileOperationCompressCancel && !s.status.State.Terminal() {
		s.status.State = model.CompressionCancelling
		s.cancel()
	}

	snapshot := s.status
	result.Compression = &snapshot

	return result, nil
}

func (m *Manager) compress(ctx context.Context, d model.Deployment, volume string, s *compressionSession) {
	defer s.cancel()

	ctx = workprogress.WithReporter(ctx, func(snapshot workprogress.Snapshot) {
		s.mu.Lock()
		s.status.WorkProgress = &snapshot
		s.mu.Unlock()
	})

	root, err := os.OpenRoot(volume)
	if err == nil {
		q := s.request
		q.Operation = model.FileOperationCompress
		_, err = m.archiveFiles(ctx, d, root, volume, q)
		_ = root.Close()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.State = model.CompressionCompleted
	if err != nil {
		s.status.State = model.CompressionFailed
		s.status.Message = ArchiveFailure(err).Error()

		switch ctx.Err() {
		case context.DeadlineExceeded:
			s.status.Message = "Compression exceeded its time limit. Try compressing fewer files."
		case context.Canceled:
			s.status.State = model.CompressionCancelled
			s.status.Message = "Compression stopped before publication."
		}
	}

	s.status.WorkProgress = nil
	s.finished = time.Now()
}
