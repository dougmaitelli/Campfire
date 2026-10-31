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
	model.TransferResult
	Role      string `json:"role"`
	Installed bool   `json:"installed"`
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

	if !utils.IsValidResourceID(transfer.ID) || transfer.ServerID != id || (transfer.Role != "source" && transfer.Role != "target") {
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

// Transfer manages durable, fenced archive movement. Export/import run under the
// Spark lifetime; reconnecting panels can safely repeat every request.
func (m *Manager) Transfer(ctx context.Context, d model.Deployment, q model.TransferRequest) (model.TransferResult, error) {
	if !m.maintenance.TryRLock() {
		return model.TransferResult{}, errors.New("node cleanup is in progress; retry after it finishes")
	}
	defer m.maintenance.RUnlock()

	if err := ctx.Err(); err != nil {
		return model.TransferResult{}, err
	}

	if !utils.IsValidResourceID(q.ID) || !utils.IsValidResourceID(q.ServerID) || d.ServerID != q.ServerID || len(q.Data) > model.FileLimit || q.Offset < 0 {
		return model.TransferResult{}, errors.New("invalid transfer request")
	}

	e, err := m.load(q.ServerID)
	if err != nil {
		return model.TransferResult{}, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	t, err := m.localTransfer(q.ServerID)
	if err != nil {
		return model.TransferResult{}, err
	}

	if t != nil && t.ID != q.ID {
		newSource := q.Operation == model.TransferOperationExport && (t.State == model.TransferStateActive || t.State == model.TransferStateReleased)

		newTarget := q.Operation == model.TransferOperationPrepare && (t.State == model.TransferStateRetired || t.State == model.TransferStateDiscarded)
		if !newSource && !newTarget {
			return model.TransferResult{}, errors.New("another transfer owns this server")
		}

		t = nil
	}

	if q.Operation == model.TransferOperationStatus {
		if t == nil {
			return model.TransferResult{ID: q.ID, ServerID: q.ServerID, State: model.TransferStateAbsent}, nil
		}

		return m.transferProgress(t)
	}

	if e.Status.Busy {
		return model.TransferResult{}, errors.New("another server operation is in progress")
	}

	if t == nil {
		t = &localTransfer{TransferResult: model.TransferResult{ID: q.ID, ServerID: q.ServerID}}
		switch q.Operation {
		case model.TransferOperationExport:
			if err := m.backupStopped(ctx, q.ServerID); err != nil {
				return model.TransferResult{}, err
			}

			t.Role, t.State, t.Installed = "source", model.TransferStateExporting, e.Installed
		case model.TransferOperationPrepare:
			if err := m.prepareTransfer(ctx, d, q, e, t); err != nil {
				return model.TransferResult{}, err
			}
		case model.TransferOperationRelease:
			t.Role, t.State = "source", model.TransferStateReleased
		case model.TransferOperationDiscard:
			t.Role, t.State = "target", model.TransferStateDiscarded
		default:
			return model.TransferResult{}, errors.New("prepare the transfer first")
		}

		if err := m.saveTransfer(t); err != nil {
			return model.TransferResult{}, err
		}

		if q.Operation == model.TransferOperationRelease || q.Operation == model.TransferOperationDiscard {
			return t.TransferResult, nil
		}
	}

	switch q.Operation {
	case model.TransferOperationExport:
		if t.Role != "source" {
			return model.TransferResult{}, errors.New("transfer role mismatch")
		}

		if t.State == model.TransferStateReady || t.State == model.TransferStateReleased || t.State == model.TransferStateRetired {
			return m.transferProgress(t)
		}

		return m.launchTransfer(d, e, t, model.TransferStateExporting)
	case model.TransferOperationPrepare:
		if t.Role != "target" || t.Size != q.Size || t.Digest != q.Digest {
			return model.TransferResult{}, errors.New("transfer archive identity mismatch")
		}

		if t.State == model.TransferStateCorrupt {
			file, err := os.OpenFile(m.transferArchive(t.ServerID, t.ID), os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				return model.TransferResult{}, err
			}

			err = file.Sync()
			_ = file.Close()

			if err != nil {
				return model.TransferResult{}, err
			}

			t.State, t.Error = model.TransferStateReceiving, ""
			if err := m.saveTransfer(t); err != nil {
				return model.TransferResult{}, err
			}
		}

		return m.transferProgress(t)
	case model.TransferOperationWrite:
		return m.writeTransfer(t, q)
	case model.TransferOperationRead:
		return m.readTransfer(t, q)
	case model.TransferOperationImport:
		if t.Role != "target" {
			return model.TransferResult{}, errors.New("transfer role mismatch")
		}

		if t.State == model.TransferStateImported || t.State == model.TransferStateActive || t.State == model.TransferStateDiscarded || t.State == model.TransferStateCorrupt {
			return m.transferProgress(t)
		}

		return m.launchTransfer(d, e, t, model.TransferStateImporting)
	case model.TransferOperationActivate:
		if t.Role != "target" || (t.State != model.TransferStateImported && t.State != model.TransferStateActive) {
			return model.TransferResult{}, errors.New("transfer is not imported")
		}

		t.State, t.Error = model.TransferStateActive, ""
		if err := m.saveTransfer(t); err != nil {
			return model.TransferResult{}, err
		}

		if err := os.Remove(m.transferArchive(t.ServerID, t.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return model.TransferResult{}, err
		}

		return t.TransferResult, nil
	case model.TransferOperationRelease, model.TransferOperationDiscard, model.TransferOperationCleanup:
		return m.finishTransfer(ctx, d, e, t, q.Operation)
	default:
		return model.TransferResult{}, errors.New("unsupported transfer operation")
	}
}

func (m *Manager) prepareTransfer(ctx context.Context, d model.Deployment, q model.TransferRequest, e *entry, t *localTransfer) error {
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

	t.Role, t.State, t.Size, t.Digest = "target", model.TransferStateReceiving, q.Size, q.Digest

	return nil
}

func (m *Manager) transferProgress(t *localTransfer) (model.TransferResult, error) {
	result := t.TransferResult
	if t.Role == "target" && (t.State == model.TransferStateReceiving || t.State == model.TransferStateFailed || t.State == model.TransferStateImporting) {
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

func (m *Manager) writeTransfer(t *localTransfer, q model.TransferRequest) (model.TransferResult, error) {
	if t.Role != "target" || t.State != "receiving" || len(q.Data) == 0 || q.Offset > t.Size-int64(len(q.Data)) {
		return model.TransferResult{}, errors.New("invalid transfer write")
	}

	file, err := os.OpenFile(m.transferArchive(t.ServerID, t.ID), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return model.TransferResult{}, err
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return model.TransferResult{}, err
	}

	if q.Offset > info.Size() {
		return model.TransferResult{}, errors.New("transfer write would leave a gap")
	}

	if q.Offset < info.Size() {
		if q.Offset+int64(len(q.Data)) > info.Size() {
			return model.TransferResult{}, errors.New("overlapping transfer write")
		}

		previous := make([]byte, len(q.Data))
		if _, err := file.ReadAt(previous, q.Offset); err != nil || !bytes.Equal(previous, q.Data) {
			return model.TransferResult{}, errors.New("replayed transfer bytes differ")
		}
	} else {
		if _, err := file.WriteAt(q.Data, q.Offset); err != nil {
			return model.TransferResult{}, err
		}

		if err := file.Sync(); err != nil {
			return model.TransferResult{}, err
		}
	}

	return m.transferProgress(t)
}

func (m *Manager) readTransfer(t *localTransfer, q model.TransferRequest) (model.TransferResult, error) {
	if t.Role != "source" || t.State != "ready" || q.Offset > t.Size {
		return model.TransferResult{}, errors.New("transfer archive is not ready")
	}

	file, err := os.Open(m.transferArchive(t.ServerID, t.ID))
	if err != nil {
		return model.TransferResult{}, err
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != t.Size {
		return model.TransferResult{}, errors.New("transfer archive changed")
	}

	result := t.TransferResult

	result.Data = make([]byte, min(int64(model.FileLimit), t.Size-q.Offset))
	if _, err := file.ReadAt(result.Data, q.Offset); err != nil {
		return model.TransferResult{}, err
	}

	result.Offset = q.Offset + int64(len(result.Data))

	return result, nil
}

func (m *Manager) launchTransfer(d model.Deployment, e *entry, t *localTransfer, phase model.TransferState) (model.TransferResult, error) {
	if m.taskRunning(d.ServerID, false) {
		return t.TransferResult, nil
	}

	if phase == model.TransferStateImporting {
		progress, err := m.transferProgress(t)
		if err != nil || progress.Offset != t.Size {
			return model.TransferResult{}, errors.New("incoming transfer archive is incomplete")
		}
	}

	t.State, t.Error = phase, ""
	if err := m.saveTransfer(t); err != nil {
		return model.TransferResult{}, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return model.TransferResult{}, errors.New("runtime is shutting down")
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
		if phase == model.TransferStateImporting {
			err = m.importTransfer(ctx, d, &work)
		} else {
			err = m.exportTransfer(ctx, d, &work)
		}

		e.mu.Lock()
		defer e.mu.Unlock()

		if err != nil {
			work.State, work.Error = model.TransferStateFailed, fmt.Sprintf("Transfer failed: %.900s", err.Error())
		}

		if errors.Is(err, errTransferChecksum) {
			work.State = model.TransferStateCorrupt
		}

		if err == nil && phase == model.TransferStateImporting {
			e.Installed = work.Installed

			e.Status = model.RuntimeStatus{ServerID: d.ServerID, State: model.RuntimeStateStopped}
			if err := m.persist(d.ServerID, e); err != nil {
				work.State, work.Error = model.TransferStateFailed, "Could not persist imported runtime state."
			}
		}

		_ = m.saveTransfer(&work)
		m.mu.Lock()
		delete(m.transferTasks, d.ServerID)
		m.mu.Unlock()
	}()

	return t.TransferResult, nil
}

func (m *Manager) finishTransfer(ctx context.Context, d model.Deployment, e *entry, t *localTransfer, operation model.TransferOperation) (model.TransferResult, error) {
	if (operation == model.TransferOperationRelease || operation == model.TransferOperationCleanup) && t.Role != "source" || operation == model.TransferOperationDiscard && t.Role != "target" {
		return model.TransferResult{}, errors.New("transfer role mismatch")
	}

	if t.State == model.TransferStateActive {
		return model.TransferResult{}, errors.New("committed destination cannot be discarded")
	}

	terminal := map[model.TransferOperation]model.TransferState{model.TransferOperationRelease: model.TransferStateReleased, model.TransferOperationDiscard: model.TransferStateDiscarded, model.TransferOperationCleanup: model.TransferStateRetired}[operation]
	if t.State == terminal {
		return t.TransferResult, nil
	}

	if t.State == model.TransferStateReleased || t.State == model.TransferStateRetired || t.State == model.TransferStateDiscarded {
		return model.TransferResult{}, errors.New("transfer has already finished")
	}

	if m.taskRunning(d.ServerID, true) {
		return t.TransferResult, nil
	}

	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return model.TransferResult{}, err
	}

	if operation != model.TransferOperationRelease {
		for _, name := range []string{container(d.ServerID), container(d.ServerID) + "-install"} {
			output, err := m.Docker.Run(ctx, nil, "rm", "--force", name)
			if err != nil && !bytes.Contains(bytes.ToLower([]byte(output)), []byte("no such")) {
				return model.TransferResult{}, err
			}
		}

		for _, name := range []string{"data", "backups"} {
			if err := os.RemoveAll(filepath.Join(m.root(d.ServerID), name)); err != nil {
				return model.TransferResult{}, err
			}
		}

		e.Installed, e.Revision = false, ""

		e.Status = model.RuntimeStatus{ServerID: d.ServerID, State: model.RuntimeStateNotInstalled}
		if err := m.persist(d.ServerID, e); err != nil {
			return model.TransferResult{}, err
		}
	}

	if err := os.Remove(m.transferArchive(t.ServerID, t.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return model.TransferResult{}, err
	}

	t.State = terminal

	t.Error = ""
	if err := m.saveTransfer(t); err != nil {
		return model.TransferResult{}, fmt.Errorf("save completed transfer: %w", err)
	}

	return t.TransferResult, nil
}
