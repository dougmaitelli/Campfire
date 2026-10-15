package runtime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"campfire/internal/shared/model"
)

type fileUpload struct {
	path, temporary, revision string
	size                      int64
	limit                     int64
	mode                      fs.FileMode
	touched                   time.Time
	ranges                    [][2]int64
}

func statRevision(info os.FileInfo) string {
	return fmt.Sprintf("%x", sha256.Sum256(fmt.Appendf(nil, "%d:%d", info.Size(), info.ModTime().UnixNano())))
}

func chunkFile(root *os.Root, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: q.ServerID, Path: q.Path}

	file, err := root.OpenFile(q.Path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return result, fileFailure("file_not_found", "File not found.")
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
		return result, fileFailure("invalid_file", "Select a regular file or directory.")
	}

	result.Size, result.Revision = info.Size(), statRevision(info)

	kind := "file"
	if info.IsDir() {
		kind = "directory"
	}

	result.Entries = []model.FileEntry{{Name: info.Name(), Kind: kind, Size: info.Size(), ModifiedAt: info.ModTime().UTC()}}
	if q.Operation == "stat" {
		return result, nil
	}

	if !info.Mode().IsRegular() || q.Offset < 0 || q.Offset > info.Size() || info.Size() > model.FileTransferLimit {
		return result, fileFailure("file_limit", "Invalid download offset or file exceeds 100 GiB.")
	}

	if q.Revision == "" || q.Revision != result.Revision {
		return result, fileFailure("file_conflict", "File changed; restart the download.")
	}

	count := int64(model.FileLimit)
	if q.Size > 0 && q.Size < count {
		count = q.Size
	}

	count = min(count, info.Size()-q.Offset)

	result.Data = make([]byte, int(count))
	if _, err := file.ReadAt(result.Data, q.Offset); err != nil && (err != io.EOF || count != 0) {
		return result, fileFailure("file_io", "Could not read download chunk.")
	}

	after, err := file.Stat()
	if err != nil || statRevision(after) != result.Revision {
		return result, fileFailure("file_conflict", "File changed; restart the download.")
	}

	return result, nil
}

func (m *Manager) pruneUploads(root *os.Root, e *entry) {
	for id, item := range e.uploads {
		if time.Since(item.touched) > time.Hour {
			_ = root.Remove(item.temporary)

			delete(e.uploads, id)
		}
	}
	// Recover abandoned staging files after a Spark restart, without following links.
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return
	}

	for _, item := range entries {
		if !strings.HasPrefix(item.Name(), ".campfire-upload-") && !strings.HasPrefix(item.Name(), ".campfire-archive-") {
			continue
		}

		id := strings.TrimPrefix(item.Name(), ".campfire-upload-")
		if e.uploads[id] != nil {
			continue
		}

		info, err := item.Info()
		if err == nil && time.Since(info.ModTime()) > time.Hour {
			_ = root.RemoveAll(item.Name())
		}
	}
}

func (m *Manager) uploadFile(ctx context.Context, d model.Deployment, root *os.Root, volume string, e *entry, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: q.ServerID, Path: q.Path, UploadID: q.UploadID}

	if e.uploads == nil {
		e.uploads = make(map[string]*fileUpload)
	}

	if q.Operation == "upload.begin" {
		limit := model.DefaultUploadLimit
		if q.UploadLimit > 0 {
			limit = min(q.UploadLimit, model.FileTransferLimit)
		}

		if len(e.uploads) >= 8 {
			return result, fileFailure("file_busy", "At most eight uploads may be open per server.")
		}

		if q.Size < -1 || q.Size > limit || !filePath(q.Path, false) {
			return result, fileFailure("file_limit", "Upload exceeds the configured maximum file size.")
		}

		if err := containedPath(root, q.Path, true); err != nil {
			return result, err
		}

		revision, mode := "absent", fs.FileMode(0600)

		if info, err := root.Lstat(q.Path); err == nil {
			if !info.Mode().IsRegular() {
				return result, fileFailure("invalid_file", "Select a regular file.")
			}

			revision, mode = statRevision(info), info.Mode().Perm()
		} else if !os.IsNotExist(err) {
			return result, fileFailure("file_io", "Could not inspect upload destination.")
		}

		if q.Revision != revision {
			return result, fileFailure("file_conflict", "Destination changed or already exists.")
		}

		if err := checkWriteCapacity(ctx, volume, d.Configuration.DiskMB, 0, max(0, q.Size)); err != nil {
			return result, err
		}

		id := rand.Text()
		temporary := ".campfire-upload-" + id

		file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return result, fileFailure("file_io", "Could not prepare upload.")
		}

		_ = file.Close()
		e.uploads[id] = &fileUpload{path: q.Path, temporary: temporary, revision: revision, size: q.Size, limit: limit, mode: mode, touched: time.Now()}
		result.UploadID = id

		return result, nil
	}

	upload := e.uploads[q.UploadID]
	if upload == nil || upload.path != q.Path {
		return result, fileFailure("file_not_found", "Upload expired or is unavailable.")
	}

	upload.touched = time.Now()

	limit := upload.limit
	if q.UploadLimit > 0 {
		limit = min(limit, q.UploadLimit)
	}

	if q.Operation == "upload.cancel" {
		_ = root.Remove(upload.temporary)

		delete(e.uploads, q.UploadID)

		return result, nil
	}

	if err := containedPath(root, upload.temporary, false); err != nil {
		return result, err
	}

	file, err := root.OpenFile(upload.temporary, os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return result, fileFailure("file_io", "Upload staging file unavailable.")
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return result, fileFailure("invalid_file", "Invalid upload staging file.")
	}

	if q.Operation == "upload.chunk" {
		if q.Offset < 0 || q.Offset > limit-int64(len(q.Data)) || upload.size >= 0 && q.Offset+int64(len(q.Data)) > upload.size {
			return result, fileFailure("file_limit", "Upload chunk exceeds the declared size or configured upload limit.")
		}

		newSize := max(info.Size(), q.Offset+int64(len(q.Data)))
		// The prepared project quota enforces total usage on every write. Avoid
		// walking the full game tree for each SFTP packet; verify logical usage
		// at admission and publication, and bound sparse-file growth here.
		if newSize > d.Configuration.DiskMB*1048576 {
			return result, fileFailure("file_limit", "Upload exceeds the server disk allocation.")
		}

		if _, err := file.WriteAt(q.Data, q.Offset); err != nil {
			return result, fileFailure("file_io", "Could not write upload chunk.")
		}

		if len(q.Data) > 0 {
			interval := [2]int64{q.Offset, q.Offset + int64(len(q.Data))}

			merged := make([][2]int64, 0, len(upload.ranges)+1)
			for _, current := range upload.ranges {
				if current[1] < interval[0] {
					merged = append(merged, current)
				} else if interval[1] < current[0] {
					merged = append(merged, interval)
					interval = current
				} else {
					interval[0] = min(interval[0], current[0])
					interval[1] = max(interval[1], current[1])
				}
			}

			merged = append(merged, interval)
			if len(merged) > 100000 {
				return result, fileFailure("file_limit", "Upload has too many disjoint ranges.")
			}

			upload.ranges = merged
		}

		result.Size = newSize

		return result, nil
	}

	if upload.size >= 0 && (info.Size() != upload.size || upload.size > 0 && (len(upload.ranges) != 1 || upload.ranges[0][0] != 0 || upload.ranges[0][1] != upload.size)) {
		return result, fileFailure("file_conflict", "Upload is incomplete.")
	}

	if info.Size() > limit {
		return result, fileFailure("file_limit", "Upload exceeds the configured maximum file size.")
	}

	if err := containedPath(root, q.Path, true); err != nil {
		return result, err
	}

	revision := "absent"
	if current, err := root.Lstat(q.Path); err == nil {
		revision = statRevision(current)
	} else if !os.IsNotExist(err) {
		return result, fileFailure("file_io", "Could not inspect upload destination.")
	}

	if revision != upload.revision {
		return result, fileFailure("file_conflict", "Destination changed during upload.")
	}

	if err := checkWriteCapacity(ctx, volume, d.Configuration.DiskMB, 0, 0); err != nil {
		return result, err
	}

	if m.FileOwner(file, d.Ember.User) != nil || file.Chmod(upload.mode) != nil || file.Sync() != nil {
		return result, fileFailure("file_io", "Could not finalize upload ownership or contents.")
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	if err := root.Rename(upload.temporary, q.Path); err != nil {
		return result, fileFailure("file_io", "Could not publish upload.")
	}

	delete(e.uploads, q.UploadID)

	result.Size = info.Size()

	return result, nil
}

func reservedFilePath(value string) bool {
	for _, part := range strings.Split(path.Clean(value), "/") {
		if strings.HasPrefix(part, ".campfire-upload-") || strings.HasPrefix(part, ".campfire-archive-") {
			return true
		}
	}

	return false
}
