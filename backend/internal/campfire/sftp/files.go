package sftp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	accessperm "campfire/internal/campfire/permissions"
	"campfire/internal/shared/model"

	"github.com/pkg/sftp"
)

type fileHandler struct {
	server    *Server
	ctx       context.Context
	token, id string
}

func (h *fileHandler) call(q model.FileRequest) (model.FileResult, error) {
	result, _, err := h.callAudited(q, "", false)
	return result, err
}

func (h *fileHandler) callAudited(q model.FileRequest, action model.AuditAction, hold bool) (result model.FileResult, audit *fileAudit, err error) {
	ctx, cancel := context.WithTimeout(h.ctx, 15*time.Second)
	defer cancel()

	quiet := action == "" && (q.Operation == model.FileOperationList || q.Operation == model.FileOperationStat || q.Operation == model.FileOperationReadChunk || q.Operation == model.FileOperationUploadChunk)
	if action == "" {
		action = model.ResourceAuditAction("server.files.sftp", string(q.Operation))
	}

	audit = h.newAudit(action)

	defer func() {
		if err != nil || !hold {
			if auditErr := audit.finish(err, true); err == nil && auditErr != nil {
				err = sftp.ErrSSHFxFailure
			}
		}
	}()

	principal, err := h.server.Auth.AuthenticateAPIToken(ctx, h.token)
	if err != nil {
		return model.FileResult{}, audit, os.ErrPermission
	}

	audit.actor(principal)

	if !quiet {
		if err := audit.begin(); err != nil {
			return model.FileResult{}, audit, sftp.ErrSSHFxFailure
		}
	}

	permissions, err := h.server.Auth.Access().ServerPermissions(ctx, principal, h.id)

	required := accessperm.File(q.Operation)
	if err != nil || required == "" || !slices.Contains(permissions, accessperm.ServerView) || !slices.Contains(permissions, required) {
		return model.FileResult{}, audit, os.ErrPermission
	}

	for _, name := range []string{q.Path, q.Destination} {
		if strings.Contains(name, "\\") || strings.ContainsRune(name, 0) {
			return model.FileResult{}, audit, os.ErrPermission
		}

		for _, part := range strings.Split(name, "/") {
			if part == ".." {
				return model.FileResult{}, audit, os.ErrPermission
			}
		}
	}

	q.Path = strings.TrimPrefix(path.Clean("/"+q.Path), "/")
	if q.Path == "" {
		q.Path = "."
	}

	if q.Destination != "" {
		q.Destination = strings.TrimPrefix(path.Clean("/"+q.Destination), "/")
	}

	result, err = h.server.Deployment.Files(ctx, h.id, q)

	return result, audit, safeError(err)
}

func (h *fileHandler) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	info, audit, err := h.callAudited(model.FileRequest{Operation: model.FileOperationStat, Path: r.Filepath}, "server.files.sftp.download", true)
	if err != nil {
		return nil, err
	}

	if len(info.Entries) != 1 || info.Entries[0].Kind != model.FileKindFile || info.Size < 0 || info.Size > model.FileTransferLimit {
		_ = audit.finish(sftp.ErrSSHFxFailure, false)
		return nil, sftp.ErrSSHFxFailure
	}

	return &remoteFile{handler: h, path: r.Filepath, revision: info.Revision, size: info.Size, audit: audit}, nil
}

func (h *fileHandler) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	flags := r.Pflags()
	if flags.Append || !flags.Write {
		return nil, sftp.ErrSSHFxOpUnsupported
	}

	revision := "absent"

	info, err := h.call(model.FileRequest{Operation: model.FileOperationStat, Path: r.Filepath})
	if err == nil {
		if flags.Excl || !flags.Trunc {
			return nil, sftp.ErrSSHFxOpUnsupported
		}

		revision = info.Revision
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else if !flags.Creat {
		return nil, os.ErrNotExist
	}

	result, err := h.call(model.FileRequest{Operation: model.FileOperationUploadBegin, Path: r.Filepath, Revision: revision, Size: -1})
	if err != nil {
		return nil, err
	}

	return &remoteFile{handler: h, path: r.Filepath, upload: result.UploadID}, nil
}

func (h *fileHandler) Filecmd(r *sftp.Request) error {
	var operation model.FileOperation

	switch r.Method {
	case "Mkdir":
		operation = model.FileOperationMkdir
	case "Rename":
		operation = model.FileOperationRename
	case "Remove", "Rmdir":
		operation = model.FileOperationDelete
	default:
		return sftp.ErrSSHFxOpUnsupported
	}

	_, err := h.call(model.FileRequest{Operation: operation, Path: r.Filepath, Destination: r.Target})

	return err
}

func (h *fileHandler) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	operation := model.FileOperationStat
	if r.Method == "List" {
		operation = model.FileOperationList
	} else if r.Method != "Stat" && r.Method != "Lstat" {
		return nil, sftp.ErrSSHFxOpUnsupported
	}

	result, err := h.call(model.FileRequest{Operation: operation, Path: r.Filepath})
	if err != nil {
		return nil, err
	}

	items := make(fileListing, 0, len(result.Entries))
	for _, item := range result.Entries {
		items = append(items, fileInfo{item})
	}

	return items, nil
}

type fileInfo struct{ model.FileEntry }

func (f fileInfo) Name() string { return f.FileEntry.Name }
func (f fileInfo) Size() int64  { return f.FileEntry.Size }
func (f fileInfo) Mode() fs.FileMode {
	if f.Kind == model.FileKindDirectory {
		return fs.ModeDir | 0700
	}

	if f.Kind == model.FileKindUnsupported {
		return fs.ModeIrregular
	}

	return 0600
}
func (f fileInfo) ModTime() time.Time { return f.ModifiedAt }
func (f fileInfo) IsDir() bool        { return f.Kind == model.FileKindDirectory }
func (f fileInfo) Sys() any           { return nil }

type fileListing []os.FileInfo

func (f fileListing) ListAt(items []os.FileInfo, offset int64) (int, error) {
	if offset < 0 {
		return 0, sftp.ErrSSHFxFailure
	}

	if offset >= int64(len(f)) {
		return 0, io.EOF
	}

	n := copy(items, f[offset:])
	if n < len(items) {
		return n, io.EOF
	}

	return n, nil
}

type remoteFile struct {
	mu                     sync.Mutex
	handler                *fileHandler
	path, revision, upload string
	size                   int64
	audit                  *fileAudit
	coverage               readCoverage
	failed, closed         bool
}

func (f *remoteFile) ReadAt(data []byte, offset int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed || offset < 0 {
		return 0, os.ErrInvalid
	}

	if len(data) == 0 {
		return 0, nil
	}

	read := 0
	for read < len(data) {
		// Speculative SFTP reads beyond EOF still authenticate against the live file.
		result, err := f.handler.call(model.FileRequest{Operation: model.FileOperationReadChunk, Path: f.path, Revision: f.revision, Offset: min(offset, f.size), Size: int64(min(len(data)-read, model.FileLimit))})
		if err != nil {
			f.failed = true
			return read, err
		}

		if len(result.Data) == 0 {
			if offset < f.size {
				f.failed = true
			}

			return read, io.EOF
		}

		n := copy(data[read:], result.Data)
		f.coverage.add(offset, offset+int64(n))
		read += n
		offset += int64(n)
	}

	return read, nil
}

func (f *remoteFile) WriteAt(data []byte, offset int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed || f.failed || offset < 0 {
		return 0, os.ErrInvalid
	}

	written := 0

	for len(data) > 0 {
		count := min(len(data), model.FileLimit)

		_, err := f.handler.call(model.FileRequest{Operation: model.FileOperationUploadChunk, Path: f.path, UploadID: f.upload, Offset: offset, Data: data[:count]})
		if err != nil {
			f.failed = true
			return written, err
		}

		written += count
		offset += int64(count)
		data = data[count:]
	}

	return written, nil
}

func (f *remoteFile) TransferError(_ error) { f.mu.Lock(); f.failed = true; f.mu.Unlock() }

func (f *remoteFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil
	}

	f.closed = true
	if f.upload == "" {
		return f.audit.finish(nil, !f.failed && f.coverage.complete(f.size))
	}

	operation := model.FileOperationUploadCommit
	if f.failed {
		operation = model.FileOperationUploadCancel
	}

	_, err := f.handler.call(model.FileRequest{Operation: operation, Path: f.path, UploadID: f.upload})
	if err != nil {
		_, _ = f.handler.call(model.FileRequest{Operation: model.FileOperationUploadCancel, Path: f.path, UploadID: f.upload})
	}

	return err
}
