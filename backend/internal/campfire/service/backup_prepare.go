package service

import (
	"context"
	"crypto/rand"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

// Prepare validates a stored archive without re-uploading or consuming it. Only
// short-lived creation/publication receipts are held in memory; bytes remain in
// their durable store. Authorization is rechecked by read for every chunk.
func (s *BackupImports) Prepare(ctx context.Context, owner int64, row database.Backup, read func(context.Context, int64) (model.FileResult, error)) (string, error) {
	if !row.Portable || row.Size < 1 || row.Size > model.FileTransferLimit {
		return "", importError("This archive cannot create a server.")
	}

	s.mu.Lock()
	count := 0

	for _, upload := range s.uploads {
		if upload.owner == owner {
			count++
		}
	}

	if len(s.uploads) >= backupUploadSessions || count >= backupUploadsPerAccount {
		s.mu.Unlock()
		return "", importError("Too many backup sessions are active.")
	}

	token := rand.Text()
	u := &BackupUpload{owner: owner, size: row.Size, store: s, BackupID: row.ID, FromLibrary: true, source: read}
	// Register before inspection so concurrent preparations share admission limits.
	s.uploads[token] = u
	u.mu.Lock()
	s.mu.Unlock()

	manifest, err := backuparchive.Inspect(ctx, &backupChunkReader{ctx: ctx, size: row.Size, read: read})
	if err == nil {
		u.Manifest = manifest
		u.Name = manifest.Mode.ArchiveName(rand.Text())
		u.timer = time.AfterFunc(backupUploadLifetime, func() { s.Cancel(owner, token) })
	}
	u.mu.Unlock()

	if err != nil {
		s.Cancel(owner, token)
		return "", importError(err.Error())
	}

	return token, nil
}
