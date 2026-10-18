package sftp

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"github.com/pkg/sftp"
)

type fileHandler struct {
	server    *Server
	ctx       context.Context
	token, id string
}

func (h *fileHandler) call(q model.FileRequest) (model.FileResult, error) {
	ctx, cancel := context.WithTimeout(h.ctx, 15*time.Second)
	defer cancel()

	principal, err := h.server.Auth.AuthenticateAPIToken(ctx, h.token)
	if err != nil {
		return model.FileResult{}, os.ErrPermission
	}

	permissions, err := h.server.Auth.Access().ServerPermissions(ctx, principal, h.id)
	if err != nil || !slices.Contains(permissions, "server.view") || !slices.Contains(permissions, "server.operate") {
		return model.FileResult{}, os.ErrPermission
	}

	for _, name := range []string{q.Path, q.Destination} {
		if strings.Contains(name, "\\") || strings.ContainsRune(name, 0) {
			return model.FileResult{}, os.ErrPermission
		}

		for _, part := range strings.Split(name, "/") {
			if part == ".." {
				return model.FileResult{}, os.ErrPermission
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

	var event database.AuditEvent

	audit := h.server.Audit != nil && q.Operation != "list" && q.Operation != "stat" && q.Operation != "read.chunk" && q.Operation != "upload.chunk"
	if audit {
		event = database.AuditEvent{ID: rand.Text(), UserID: &principal.UserID, Username: principal.Username, APITokenID: principal.APITokenID, Action: "server.files.sftp." + q.Operation, Target: "server:" + h.id, Outcome: "pending", CreatedAt: time.Now().Unix()}
		if err := h.server.Audit.BeginAudit(ctx, &event); err != nil {
			return model.FileResult{}, sftp.ErrSSHFxFailure
		}
	}

	result, err := h.server.Deployment.Files(ctx, h.id, q)

	if audit {
		event.Outcome, event.Status = "success", 200
		if err != nil {
			event.Outcome, event.Status = "failure", 502
		}

		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		auditErr := h.server.Audit.FinishAudit(auditCtx, event)

		cancel()

		if err == nil && auditErr != nil {
			err = sftp.ErrSSHFxFailure
		}
	}

	return result, safeError(err)
}

func (h *fileHandler) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	info, err := h.call(model.FileRequest{Operation: "stat", Path: r.Filepath})
	if err != nil {
		return nil, err
	}

	if len(info.Entries) != 1 || info.Entries[0].Kind != "file" || info.Size > model.FileTransferLimit {
		return nil, sftp.ErrSSHFxFailure
	}

	return &remoteFile{handler: h, path: r.Filepath, revision: info.Revision, size: info.Size}, nil
}

func (h *fileHandler) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	flags := r.Pflags()
	if flags.Append || !flags.Write {
		return nil, sftp.ErrSSHFxOpUnsupported
	}

	revision := "absent"

	info, err := h.call(model.FileRequest{Operation: "stat", Path: r.Filepath})
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

	result, err := h.call(model.FileRequest{Operation: "upload.begin", Path: r.Filepath, Revision: revision, Size: -1})
	if err != nil {
		return nil, err
	}

	return &remoteFile{handler: h, path: r.Filepath, upload: result.UploadID}, nil
}

func (h *fileHandler) Filecmd(r *sftp.Request) error {
	operation := ""

	switch r.Method {
	case "Mkdir":
		operation = "mkdir"
	case "Rename":
		operation = "rename"
	case "Remove", "Rmdir":
		operation = "delete"
	default:
		return sftp.ErrSSHFxOpUnsupported
	}

	_, err := h.call(model.FileRequest{Operation: operation, Path: r.Filepath, Destination: r.Target})

	return err
}

func (h *fileHandler) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	operation := "stat"
	if r.Method == "List" {
		operation = "list"
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
	if f.Kind == "directory" {
		return fs.ModeDir | 0700
	}

	if f.Kind == "unsupported" {
		return fs.ModeIrregular
	}

	return 0600
}
func (f fileInfo) ModTime() time.Time { return f.ModifiedAt }
func (f fileInfo) IsDir() bool        { return f.Kind == "directory" }
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
		result, err := f.handler.call(model.FileRequest{Operation: "read.chunk", Path: f.path, Revision: f.revision, Offset: min(offset, f.size), Size: int64(min(len(data)-read, model.FileLimit))})
		if err != nil {
			return read, err
		}

		if len(result.Data) == 0 {
			return read, io.EOF
		}

		n := copy(data[read:], result.Data)
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

		_, err := f.handler.call(model.FileRequest{Operation: "upload.chunk", Path: f.path, UploadID: f.upload, Offset: offset, Data: data[:count]})
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
		return nil
	}

	operation := "upload.commit"
	if f.failed {
		operation = "upload.cancel"
	}

	_, err := f.handler.call(model.FileRequest{Operation: operation, Path: f.path, UploadID: f.upload})
	if err != nil {
		_, _ = f.handler.call(model.FileRequest{Operation: "upload.cancel", Path: f.path, UploadID: f.upload})
	}

	return err
}
