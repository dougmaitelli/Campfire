package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"campfire/internal/shared/backuparchive"
	"campfire/internal/shared/model"
)

// publishJobBackup adapts the shared durable receiver to portable backup
// restoration. Transport identity and archive integrity are checked before the
// immutable archive is published; restore remains a separate workflow step.
func (m *Manager) publishJobBackup(ctx context.Context, d model.Deployment, work *localTransfer) error {
	if !model.BackupName(work.ArchiveName) {
		return errors.New("invalid job backup name")
	}

	name := m.transferArchive(d.ServerID, work.ID)
	file, err := os.Open(name)

	var published bool

	if errors.Is(err, os.ErrNotExist) {
		name = filepath.Join(m.root(d.ServerID), "backups", work.ArchiveName)
		file, err = os.Open(name)
		published = err == nil
	}

	if err != nil {
		return err
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	if info.Size() != work.Size {
		return errors.New("job archive size mismatch")
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, backupReader{ctx, file}); err != nil {
		return err
	}

	if hex.EncodeToString(hash.Sum(nil)) != work.Digest {
		return errTransferChecksum
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	manifest, err := backuparchive.Inspect(ctx, file)
	if err != nil {
		return err
	}

	if manifest.Mode != model.BackupModeFull || manifest.Definition == nil {
		return errors.New("copy requires a portable full backup")
	}

	directory := filepath.Join(m.root(d.ServerID), "backups")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}

	if !published {
		if err := os.Rename(name, filepath.Join(directory, work.ArchiveName)); err != nil {
			return err
		}
	}

	if err := syncBackupDirectory(directory); err != nil {
		return err
	}

	work.State = model.JobStatePublished

	return nil
}
