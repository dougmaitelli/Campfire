package fileops

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"campfire/internal/shared/model"
	uploadstore "campfire/internal/shared/upload"
)

const uploadScanInterval = time.Minute

type fileUpload struct {
	buffer                    *uploadstore.Session
	path, temporary, revision string
	mode                      fs.FileMode
	touched                   time.Time
}

// StatRevision identifies a filesystem object and its observed metadata.
func StatRevision(info os.FileInfo) string {
	identity := ""
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		identity = fmt.Sprintf("%d:%d:%d:%d", stat.Dev, stat.Ino, stat.Ctim.Sec, stat.Ctim.Nsec)
	}

	return fmt.Sprintf("%x", sha256.Sum256(fmt.Appendf(nil, "%d:%d:%d:%s", info.Size(), info.ModTime().UnixNano(), info.Mode(), identity)))
}

func chunkFile(root *os.Root, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: q.ServerID, Path: q.Path}

	file, err := root.OpenFile(q.Path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return result, Failure(model.FileNotFound, "File not found.")
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
		return result, Failure(model.FileInvalid, "Select a regular file or directory.")
	}

	result.Size, result.Revision = info.Size(), StatRevision(info)

	kind := model.FileKindFile
	if info.IsDir() {
		kind = model.FileKindDirectory
	}

	result.Entries = []model.FileEntry{{Name: info.Name(), Kind: kind, Mode: uint32(info.Mode().Perm()), Size: info.Size(), ModifiedAt: info.ModTime().UTC()}}
	if q.Operation == model.FileOperationStat {
		return result, nil
	}

	if !info.Mode().IsRegular() || q.Offset < 0 || q.Offset > info.Size() || info.Size() > model.FileTransferLimit {
		return result, Failure(model.FileLimitExceeded, "Invalid download offset or file exceeds 100 GiB.")
	}

	if q.Revision == "" || q.Revision != result.Revision {
		return result, Failure(model.FileConflict, "File changed; restart the download.")
	}

	count := int64(model.FileLimit)
	if q.Size > 0 && q.Size < count {
		count = q.Size
	}

	count = min(count, info.Size()-q.Offset)

	result.Data = make([]byte, int(count))
	if _, err := file.ReadAt(result.Data, q.Offset); err != nil && (err != io.EOF || count != 0) {
		return result, Failure(model.FileIO, "Could not read download chunk.")
	}

	after, err := file.Stat()
	if err != nil || StatRevision(after) != result.Revision {
		return result, Failure(model.FileConflict, "File changed; restart the download.")
	}

	return result, nil
}

func (m *Manager) pruneUploads(root *os.Root, e *State) {
	now := time.Now()

	for id, item := range e.uploads {
		if now.Sub(item.touched) > time.Hour {
			if item.buffer != nil {
				item.buffer.Close()
			} else {
				_ = root.Remove(item.temporary)
			}

			delete(e.uploads, id)
		}
	}

	if !e.lastUploadScan.IsZero() && now.Sub(e.lastUploadScan) < uploadScanInterval {
		return
	}

	e.lastUploadScan = now
	// Recover abandoned staging files after a Spark restart, without following links.
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return
	}

	for _, item := range entries {
		if !IsStagingName(item.Name()) {
			continue
		}

		id := strings.TrimPrefix(item.Name(), UploadStagingPrefix)
		if e.uploads[id] != nil {
			continue
		}

		info, err := item.Info()
		if err == nil && now.Sub(info.ModTime()) > time.Hour {
			_ = root.RemoveAll(item.Name())
		}
	}
}

func (m *Manager) uploadFile(ctx context.Context, d model.Deployment, root *os.Root, volume string, e *State, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: q.ServerID, Path: q.Path, UploadID: q.UploadID}

	if e.uploads == nil {
		e.uploads = make(map[string]*fileUpload)
	}

	if q.Operation == model.FileOperationUploadBegin {
		limit := model.DefaultUploadLimit
		if q.UploadLimit > 0 {
			limit = min(q.UploadLimit, model.FileTransferLimit)
		}

		if len(e.uploads) >= 8 {
			return result, Failure(model.FileBusy, "At most eight uploads may be open per server.")
		}

		if q.Size < -1 || q.Size > limit || !ValidPath(q.Path, false) {
			return result, Failure(model.FileLimitExceeded, "Upload exceeds the configured maximum file size.")
		}

		if err := ContainedPath(root, q.Path, true); err != nil {
			return result, err
		}

		revision, mode := "absent", fs.FileMode(0600)

		if info, err := root.Lstat(q.Path); err == nil {
			if !info.Mode().IsRegular() {
				return result, Failure(model.FileInvalid, "Select a regular file.")
			}

			revision, mode = StatRevision(info), info.Mode().Perm()
		} else if !os.IsNotExist(err) {
			return result, Failure(model.FileIO, "Could not inspect upload destination.")
		}

		if q.Revision != revision {
			return result, Failure(model.FileConflict, "Destination changed or already exists.")
		}

		if err := CheckWriteCapacity(ctx, volume, m.LogicalDiskLimitMB(d.Configuration.DiskMB), 0, max(0, q.Size)); err != nil {
			return result, err
		}

		id := rand.Text()
		temporary := UploadStagingPrefix + id

		buffer, err := uploadstore.CreateAt(root, temporary, q.Size, limit)
		if err != nil {
			return result, Failure(model.FileIO, "Could not prepare upload.")
		}

		e.uploads[id] = &fileUpload{buffer: buffer, path: q.Path, temporary: temporary, revision: revision, mode: mode, touched: time.Now()}
		result.UploadID = id

		return result, nil
	}

	upload := e.uploads[q.UploadID]
	if upload == nil || upload.path != q.Path {
		return result, Failure(model.FileNotFound, "Upload expired or is unavailable.")
	}

	upload.touched = time.Now()

	limit := upload.buffer.Limit
	if q.UploadLimit > 0 {
		limit = min(limit, q.UploadLimit)
	}

	if q.Operation == model.FileOperationUploadCancel {
		upload.buffer.Close()

		delete(e.uploads, q.UploadID)

		return result, nil
	}

	if err := ContainedPath(root, upload.temporary, false); err != nil {
		return result, err
	}

	file := upload.buffer.File
	info, err := file.Stat()

	current, currentErr := root.Lstat(upload.temporary)
	if err != nil || currentErr != nil || !current.Mode().IsRegular() || !os.SameFile(info, current) {
		return result, Failure(model.FileInvalid, "Invalid upload staging file.")
	}

	if q.Operation == model.FileOperationUploadChunk {
		if q.Offset < 0 || q.Offset > limit-int64(len(q.Data)) || upload.buffer.Size >= 0 && q.Offset+int64(len(q.Data)) > upload.buffer.Size {
			return result, Failure(model.FileLimitExceeded, "Upload chunk exceeds the declared size or configured upload limit.")
		}

		newSize := max(info.Size(), q.Offset+int64(len(q.Data)))
		// Without a native quota, check the tree at admission and publication
		// and bound this file's growth between those checks.
		if limitMB := m.LogicalDiskLimitMB(d.Configuration.DiskMB); limitMB > 0 && newSize > limitMB<<20 {
			return result, Failure(model.FileLimitExceeded, "Upload exceeds the server disk allocation.")
		}

		if err := upload.buffer.Write(q.Offset, q.Data); err != nil {
			if errors.Is(err, syscall.EDQUOT) {
				return result, Failure(model.FileLimitExceeded, "Upload exceeds the server disk allocation.")
			}
			if errors.Is(err, uploadstore.ErrTooManyRanges) {
				return result, Failure(model.FileLimitExceeded, "Upload has too many disjoint ranges.")
			}

			return result, Failure(model.FileIO, "Could not write upload chunk.")
		}

		result.Size = newSize

		return result, nil
	}

	if err := upload.buffer.Complete(); err != nil {
		return result, Failure(model.FileConflict, "Upload is incomplete.")
	}

	if info.Size() > limit {
		return result, Failure(model.FileLimitExceeded, "Upload exceeds the configured maximum file size.")
	}

	if err := CheckWriteCapacity(ctx, volume, m.LogicalDiskLimitMB(d.Configuration.DiskMB), 0, 0); err != nil {
		return result, err
	}

	if m.FileOwner(file, d.Ember.User) != nil || file.Chmod(upload.mode) != nil {
		return result, Failure(model.FileIO, "Could not finalize upload ownership or permissions.")
	}

	if err := file.Sync(); err != nil {
		if errors.Is(err, syscall.EDQUOT) {
			return result, Failure(model.FileLimitExceeded, "Upload exceeds the server disk allocation.")
		}

		return result, Failure(model.FileIO, "Could not flush the uploaded file.")
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	if err := PublishStagedFile(ctx, root, upload.temporary, q.Path, upload.revision); err != nil {
		return result, err
	}

	upload.buffer.Close()
	delete(e.uploads, q.UploadID)

	result.Size = info.Size()

	return result, nil
}

// ReservedPath recognizes existing private upload/archive staging paths.
func ReservedPath(value string) bool {
	for _, part := range strings.Split(path.Clean(value), "/") {
		if IsStagingName(part) {
			return true
		}
	}

	return false
}
