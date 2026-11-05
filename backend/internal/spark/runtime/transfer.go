package runtime

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

type localTransfer struct {
	model.JobResult
	ArchiveFormat string        `json:"archive_format,omitempty"`
	ArchiveName   string        `json:"archive_name,omitempty"`
	Attempt       int64         `json:"attempt"`
	Role          model.JobRole `json:"role"`
	Installed     bool          `json:"installed"`
}

func (t *localTransfer) allowsOperations() bool {
	return t.State == model.JobStateActive || t.State == model.JobStateReleased || t.ArchiveFormat == "backup" && (t.State == model.JobStatePublished || t.State == model.JobStateDiscarded)
}

func (m *Manager) localTransfer(id string) (*localTransfer, error) {
	data, err := os.ReadFile(filepath.Join(m.root(id), "transfer.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var transfer localTransfer
	if err := json.Unmarshal(data, &transfer); err != nil {
		return nil, errors.New("invalid transfer state; operator recovery required")
	}

	if !utils.IsValidResourceID(transfer.ID) || transfer.ServerID != id || (transfer.Role != model.JobSource && transfer.Role != model.JobTarget) {
		return nil, errors.New("invalid transfer identity")
	}

	return &transfer, nil
}

func (m *Manager) saveTransfer(t *localTransfer) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}

	return writeAtomic(filepath.Join(m.root(t.ServerID), "transfer.json"), data, 0600)
}

func (m *Manager) transferArchive(serverID, id string) string {
	return filepath.Join(m.root(serverID), ".transfer-"+id+".tar.gz")
}

func (m *Manager) taskRunning(id string, cancel bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	stop := m.transferTasks[id]
	if stop != nil && cancel {
		stop()
	}

	return stop != nil
}

// Job manages durable, fenced archive staging and publication.
func (m *Manager) Job(ctx context.Context, d model.Deployment, q model.JobRequest) (model.JobResult, error) {
	if !m.maintenance.TryRLock() {
		return model.JobResult{}, errors.New("node cleanup is in progress; retry after it finishes")
	}
	defer m.maintenance.RUnlock()

	if err := ctx.Err(); err != nil {
		return model.JobResult{}, err
	}

	if !utils.IsValidResourceID(q.ID) || !utils.IsValidResourceID(q.ServerID) || d.ServerID != q.ServerID || len(q.Data) > model.FileLimit || q.Offset < 0 || q.Attempt < 0 {
		return model.JobResult{}, errors.New("invalid transfer request")
	}

	e, err := m.load(q.ServerID)
	if err != nil {
		return model.JobResult{}, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	t, err := m.localTransfer(q.ServerID)
	if err != nil {
		return model.JobResult{}, err
	}

	if t != nil && t.ID != q.ID {
		newSource := q.Operation == model.JobOperationExport && (t.State == model.JobStateActive || t.State == model.JobStateReleased)

		newTarget := q.Operation == model.JobOperationPrepare && (t.State == model.JobStateRetired || t.State == model.JobStateDiscarded)
		if !newSource && !newTarget {
			return model.JobResult{}, errors.New("another transfer owns this server")
		}

		t = nil
	}

	if q.Operation == model.JobOperationStatus {
		if t == nil {
			return model.JobResult{ID: q.ID, ServerID: q.ServerID, State: model.JobStateAbsent}, nil
		}

		return m.transferProgress(t)
	}

	if e.Status.Busy {
		return model.JobResult{}, errors.New("another server operation is in progress")
	}

	if t == nil {
		t = &localTransfer{JobResult: model.JobResult{ID: q.ID, ServerID: q.ServerID}}
		switch q.Operation {
		case model.JobOperationExport:
			if err := m.backupStopped(ctx, q.ServerID); err != nil {
				return model.JobResult{}, err
			}

			t.Role, t.State, t.Installed = model.JobSource, model.JobStateExporting, e.Installed
		case model.JobOperationPrepare:
			t.ArchiveFormat, t.ArchiveName = q.ArchiveFormat, q.ArchiveName
			if err := m.prepareTransfer(ctx, d, q, e, t); err != nil {
				return model.JobResult{}, err
			}
		case model.JobOperationRelease:
			t.Role, t.State = model.JobSource, model.JobStateReleased
		case model.JobOperationDiscard:
			t.Role, t.State = model.JobTarget, model.JobStateDiscarded
			t.ArchiveFormat = q.ArchiveFormat
		default:
			return model.JobResult{}, errors.New("prepare the transfer first")
		}

		if err := m.saveTransfer(t); err != nil {
			return model.JobResult{}, err
		}

		if q.Operation == model.JobOperationRelease || q.Operation == model.JobOperationDiscard {
			return t.JobResult, nil
		}
	}

	switch q.Operation {
	case model.JobOperationExport:
		if t.Role != model.JobSource {
			return model.JobResult{}, errors.New("transfer role mismatch")
		}

		if t.State == model.JobStateReady || t.State == model.JobStateReleased || t.State == model.JobStateRetired {
			return m.transferProgress(t)
		}

		if t.State == model.JobStateFailed && q.Attempt <= t.Attempt {
			return m.transferProgress(t)
		}

		t.Attempt = q.Attempt

		return m.launchTransfer(d, e, t, model.JobStateExporting)
	case model.JobOperationPrepare:
		if t.Role != model.JobTarget || t.Size != q.Size || t.Digest != q.Digest || t.ArchiveFormat != q.ArchiveFormat || t.ArchiveName != q.ArchiveName {
			return model.JobResult{}, errors.New("transfer archive identity mismatch")
		}

		if t.State == model.JobStateCorrupt {
			file, err := os.OpenFile(m.transferArchive(t.ServerID, t.ID), os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				return model.JobResult{}, err
			}

			err = file.Sync()
			_ = file.Close()

			if err != nil {
				return model.JobResult{}, err
			}

			t.State, t.Error = model.JobStateReceiving, ""
			if err := m.saveTransfer(t); err != nil {
				return model.JobResult{}, err
			}
		}

		return m.transferProgress(t)
	case model.JobOperationWrite:
		return m.writeTransfer(t, q)
	case model.JobOperationRead:
		return m.readTransfer(t, q)
	case model.JobOperationImport:
		if t.Role != model.JobTarget {
			return model.JobResult{}, errors.New("transfer role mismatch")
		}

		if t.State == model.JobStateImported || t.State == model.JobStatePublished || t.State == model.JobStateActive || t.State == model.JobStateDiscarded || t.State == model.JobStateCorrupt {
			return m.transferProgress(t)
		}

		// Report a failed import until the panel starts a new attempt. Retrying
		// status polling must not repeatedly launch a persistently failing task.
		if t.State == model.JobStateFailed && q.Attempt <= t.Attempt {
			return m.transferProgress(t)
		}

		t.Attempt = q.Attempt

		return m.launchTransfer(d, e, t, model.JobStateImporting)
	case model.JobOperationActivate:
		if t.Role != model.JobTarget || (t.State != model.JobStateImported && t.State != model.JobStatePublished && t.State != model.JobStateActive) {
			return model.JobResult{}, errors.New("transfer is not imported")
		}

		if t.State == model.JobStatePublished && !e.Installed {
			return model.JobResult{}, errors.New("copy restore is not complete")
		}

		t.State, t.Error = model.JobStateActive, ""
		if err := m.saveTransfer(t); err != nil {
			return model.JobResult{}, err
		}

		if err := os.Remove(m.transferArchive(t.ServerID, t.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return model.JobResult{}, err
		}

		return t.JobResult, nil
	case model.JobOperationRelease, model.JobOperationDiscard, model.JobOperationCleanup:
		return m.finishTransfer(ctx, d, e, t, q.Operation)
	default:
		return model.JobResult{}, errors.New("unsupported transfer operation")
	}
}

func (m *Manager) prepareTransfer(ctx context.Context, d model.Deployment, q model.JobRequest, e *entry, t *localTransfer) error {
	if q.ArchiveFormat != "" && q.ArchiveFormat != "backup" {
		return errors.New("invalid job archive format")
	}

	if q.ArchiveFormat == "backup" && !model.BackupName(q.ArchiveName) {
		return errors.New("invalid job backup name")
	}

	if e.Installed {
		return errors.New("destination already has an installed server")
	}

	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return err
	}

	for _, name := range []string{"data", "backups", "restore-previous"} {
		items, err := os.ReadDir(filepath.Join(m.root(d.ServerID), name))
		if !errors.Is(err, os.ErrNotExist) && (err != nil || len(items) > 0) {
			return errors.New("destination already contains server data")
		}
	}

	digest, err := hex.DecodeString(q.Digest)
	if err != nil || len(digest) != 32 || q.Size < 1 || q.Size > backupBytes {
		return errors.New("invalid transfer archive bounds")
	}
	// Preflight quotas before accepting any potentially large archive.
	if err := os.MkdirAll(m.root(d.ServerID), 0700); err != nil {
		return err
	}

	probe, err := os.MkdirTemp(m.root(d.ServerID), ".quota-probe-")
	if err != nil {
		return err
	}

	defer func() { _ = os.RemoveAll(probe) }()

	if err := m.prepareQuota(probe, d.Configuration.DiskMB); err != nil {
		return err
	}

	t.Role, t.State, t.Size, t.Digest = model.JobTarget, model.JobStateReceiving, q.Size, q.Digest

	return nil
}

func (m *Manager) transferProgress(t *localTransfer) (model.JobResult, error) {
	result := t.JobResult
	if t.Role == model.JobTarget && (t.State == model.JobStateReceiving || t.State == model.JobStateFailed || t.State == model.JobStateImporting) {
		info, err := os.Stat(m.transferArchive(t.ServerID, t.ID))
		if errors.Is(err, os.ErrNotExist) {
			return result, nil
		}

		if err != nil || !info.Mode().IsRegular() || info.Size() > t.Size {
			return result, errors.New("invalid incoming archive")
		}

		result.Offset = info.Size()
	}

	return result, nil
}

func (m *Manager) writeTransfer(t *localTransfer, q model.JobRequest) (model.JobResult, error) {
	if t.Role != model.JobTarget || t.State != model.JobStateReceiving || len(q.Data) == 0 || q.Offset > t.Size-int64(len(q.Data)) {
		return model.JobResult{}, errors.New("invalid transfer write")
	}

	file, err := os.OpenFile(m.transferArchive(t.ServerID, t.ID), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return model.JobResult{}, err
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return model.JobResult{}, err
	}

	if q.Offset > info.Size() {
		return model.JobResult{}, errors.New("transfer write would leave a gap")
	}

	if q.Offset < info.Size() {
		if q.Offset+int64(len(q.Data)) > info.Size() {
			return model.JobResult{}, errors.New("overlapping transfer write")
		}

		previous := make([]byte, len(q.Data))
		if _, err := file.ReadAt(previous, q.Offset); err != nil || !bytes.Equal(previous, q.Data) {
			return model.JobResult{}, errors.New("replayed transfer bytes differ")
		}
	} else {
		if _, err := file.WriteAt(q.Data, q.Offset); err != nil {
			return model.JobResult{}, err
		}

		if err := file.Sync(); err != nil {
			return model.JobResult{}, err
		}
	}

	return m.transferProgress(t)
}

func (m *Manager) readTransfer(t *localTransfer, q model.JobRequest) (model.JobResult, error) {
	if t.Role != model.JobSource || t.State != model.JobStateReady || q.Offset > t.Size {
		return model.JobResult{}, errors.New("transfer archive is not ready")
	}

	file, err := os.Open(m.transferArchive(t.ServerID, t.ID))
	if err != nil {
		return model.JobResult{}, err
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != t.Size {
		return model.JobResult{}, errors.New("transfer archive changed")
	}

	result := t.JobResult

	result.Data = make([]byte, min(int64(model.FileLimit), t.Size-q.Offset))
	if _, err := file.ReadAt(result.Data, q.Offset); err != nil {
		return model.JobResult{}, err
	}

	result.Offset = q.Offset + int64(len(result.Data))

	return result, nil
}

func (m *Manager) launchTransfer(d model.Deployment, e *entry, t *localTransfer, phase model.JobState) (model.JobResult, error) {
	if m.taskRunning(d.ServerID, false) {
		return t.JobResult, nil
	}

	if phase == model.JobStateImporting {
		progress, err := m.transferProgress(t)
		published := false

		if t.ArchiveFormat == "backup" && model.BackupName(t.ArchiveName) {
			info, statErr := os.Stat(filepath.Join(m.root(d.ServerID), "backups", t.ArchiveName))
			published = statErr == nil && info.Mode().IsRegular() && info.Size() == t.Size
		}

		if err != nil || progress.Offset != t.Size && !published {
			return model.JobResult{}, errors.New("incoming transfer archive is incomplete")
		}
	}

	t.State, t.Error = phase, ""
	if err := m.saveTransfer(t); err != nil {
		return model.JobResult{}, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return model.JobResult{}, errors.New("runtime is shutting down")
	}

	if m.transferTasks == nil {
		m.transferTasks = make(map[string]context.CancelFunc)
	}

	ctx, cancel := context.WithTimeout(m.ctx, 45*time.Minute)
	m.transferTasks[d.ServerID] = cancel
	m.wg.Add(1)
	m.mu.Unlock()

	work := *t

	m.maintenance.RLock()

	go func() {
		defer m.maintenance.RUnlock()
		defer m.wg.Done()
		defer cancel()

		var err error

		if phase == model.JobStateImporting {
			if work.ArchiveFormat == "backup" {
				err = m.publishJobBackup(ctx, d, &work)
			} else {
				err = m.importTransfer(ctx, d, &work)
			}
		} else {
			err = m.exportTransfer(ctx, d, &work)
		}

		e.mu.Lock()
		defer e.mu.Unlock()

		if err != nil {
			work.State, work.Error = model.JobStateFailed, fmt.Sprintf("Transfer failed: %.900s", err.Error())
		}

		if errors.Is(err, errTransferChecksum) {
			work.State = model.JobStateCorrupt
		}

		if err == nil && phase == model.JobStateImporting && work.ArchiveFormat != "backup" {
			e.Installed = work.Installed

			e.Status = model.RuntimeStatus{ServerID: d.ServerID, State: model.RuntimeStateStopped}
			if err := m.persist(d.ServerID, e); err != nil {
				work.State, work.Error = model.JobStateFailed, "Could not persist imported runtime state."
			}
		}

		_ = m.saveTransfer(&work)
		m.mu.Lock()
		delete(m.transferTasks, d.ServerID)
		m.mu.Unlock()
	}()

	return t.JobResult, nil
}

func (m *Manager) finishTransfer(ctx context.Context, d model.Deployment, e *entry, t *localTransfer, operation model.JobOperation) (model.JobResult, error) {
	if (operation == model.JobOperationRelease || operation == model.JobOperationCleanup) && t.Role != model.JobSource || operation == model.JobOperationDiscard && t.Role != model.JobTarget {
		return model.JobResult{}, errors.New("transfer role mismatch")
	}

	if t.State == model.JobStateActive {
		return model.JobResult{}, errors.New("committed destination cannot be discarded")
	}

	terminal := map[model.JobOperation]model.JobState{model.JobOperationRelease: model.JobStateReleased, model.JobOperationDiscard: model.JobStateDiscarded, model.JobOperationCleanup: model.JobStateRetired}[operation]
	if t.State == terminal {
		return t.JobResult, nil
	}

	if t.State == model.JobStateReleased || t.State == model.JobStateRetired || t.State == model.JobStateDiscarded {
		return model.JobResult{}, errors.New("transfer has already finished")
	}

	if m.taskRunning(d.ServerID, true) {
		return t.JobResult, nil
	}

	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return model.JobResult{}, err
	}

	if err := m.removeTransferStaging(d.ServerID, t.ID); err != nil {
		return model.JobResult{}, err
	}

	if operation != model.JobOperationRelease {
		for _, name := range []string{container(d.ServerID), container(d.ServerID) + "-install"} {
			output, err := m.Docker.Run(ctx, nil, "rm", "--force", name)
			if err != nil && !bytes.Contains(bytes.ToLower([]byte(output)), []byte("no such")) {
				return model.JobResult{}, err
			}
		}

		for _, name := range []string{"data", "backups"} {
			if err := os.RemoveAll(filepath.Join(m.root(d.ServerID), name)); err != nil {
				return model.JobResult{}, err
			}
		}

		e.Installed, e.Revision = false, ""

		e.Status = model.RuntimeStatus{ServerID: d.ServerID, State: model.RuntimeStateNotInstalled}
		if err := m.persist(d.ServerID, e); err != nil {
			return model.JobResult{}, err
		}
	}

	if err := os.Remove(m.transferArchive(t.ServerID, t.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return model.JobResult{}, err
	}

	t.State = terminal

	t.Error = ""
	if err := m.saveTransfer(t); err != nil {
		return model.JobResult{}, fmt.Errorf("save completed transfer: %w", err)
	}

	return t.JobResult, nil
}
