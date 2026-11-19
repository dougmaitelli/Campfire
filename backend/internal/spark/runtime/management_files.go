package runtime

import (
	"context"

	"campfire/internal/shared/model"
	"campfire/internal/spark/runtime/fileops"
)

// Files applies runtime lifecycle/transfer/quota policy before dispatching to
// fileops, which owns filesystem implementation and file-operation state.
func (m *Manager) Files(ctx context.Context, d model.Deployment, q model.FileRequest) (model.FileResult, error) {
	if q.Operation.IsBackup() {
		return m.backupFiles(ctx, d, q)
	}

	if err := ctx.Err(); err != nil {
		return model.FileResult{}, err
	}

	if q.ServerID != d.ServerID {
		return model.FileResult{}, fileFailure(model.FileInvalid, "Invalid file request.")
	}

	e, err := m.load(q.ServerID)
	if err != nil {
		return model.FileResult{}, fileFailure(model.FileIO, "Runtime unavailable.")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Status.FileSystemBusy && !q.Operation.IsFileControl() {
		return model.FileResult{}, fileFailure(model.FileBusy, "Wait for the current server operation to finish.")
	}

	mutate := q.Operation != model.FileOperationList && q.Operation != model.FileOperationRead && q.Operation != model.FileOperationStat && q.Operation != model.FileOperationReadChunk && q.Operation != model.FileOperationUploadCancel && !q.Operation.IsFileControl()
	if mutate {
		if transfer, err := m.localTransfer(q.ServerID); err != nil || transfer != nil && !transfer.allowsOperations() {
			return model.FileResult{}, fileFailure(model.FileBusy, "Server files are locked for a Spark transfer.")
		}
	}

	volume, err := m.volume(q.ServerID)
	if err != nil {
		return model.FileResult{}, fileFailure(model.FileIO, "Game data unavailable.")
	}

	if mutate && (q.Operation == model.FileOperationWrite || q.Operation == model.FileOperationMkdir || q.Operation.IsUpload() || q.Operation.IsArchive() || q.Operation == model.FileOperationCompressStart) {
		if err := m.prepareQuota(volume, d.Configuration.DiskMB); err != nil {
			return model.FileResult{}, fileFailure(model.FileLimitExceeded, err.Error())
		}
	}

	if e.files == nil {
		e.files = &fileops.State{}
	}

	return m.fileOperations.Files(ctx, d, volume, e.files, q)
}

// Other runtime domains (backups, configuration and server transfers) use the
// same filesystem primitives. These aliases contain no duplicate implementation.
var (
	fileFailure     = fileops.Failure
	filePath        = fileops.ValidPath
	containedPath   = fileops.ContainedPath
	assignFileOwner = fileops.AssignFileOwner
	importedMode    = fileops.ImportedMode
)
