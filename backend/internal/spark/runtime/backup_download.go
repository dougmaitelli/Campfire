package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"campfire/internal/shared/model"
)

func (m *Manager) downloadBackup(ctx context.Context, d model.Deployment, name string, source *model.BackupSource, e *entry) error {
	if !backupName(name) || source.Size < 1 || source.Size > backupBytes || len(source.Digest) != 64 {
		return errors.New("invalid off-host backup")
	}

	if _, err := hex.DecodeString(source.Digest); err != nil {
		return errors.New("invalid backup checksum")
	}

	target, err := url.Parse(source.URL)
	if err != nil || (target.Scheme != "https" && target.Scheme != "http") || target.Host == "" || target.User != nil || target.Fragment != "" {
		return errors.New("invalid backup download endpoint")
	}

	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return err
	}

	m.progress(d.ServerID, e, "Downloading and verifying off-host backup")

	directory := filepath.Join(m.root(d.ServerID), "backups")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return errors.New("could not prepare backup storage")
	}

	file, err := os.CreateTemp(directory, ".offhost-")
	if err != nil {
		return errors.New("could not prepare backup download")
	}

	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return errors.New("could not prepare backup request")
	}

	client := &http.Client{Timeout: 45 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	response, err := client.Do(request)
	if err != nil {
		return errors.New("off-host backup download failed; verify storage availability and retry")
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return errors.New("off-host backup download was rejected; retry to refresh access")
	}

	hash := sha256.New()

	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, source.Size+1))
	if err != nil || written != source.Size || hex.EncodeToString(hash.Sum(nil)) != source.Digest {
		return errors.New("off-host backup failed size or checksum validation; original game data was preserved")
	}

	if err := file.Sync(); err != nil {
		return errors.New("could not synchronize downloaded backup")
	}

	if err := file.Close(); err != nil {
		return errors.New("could not save downloaded backup")
	}

	if err := os.Rename(file.Name(), filepath.Join(directory, name)); err != nil {
		return errors.New("could not publish downloaded backup")
	}

	return syncBackupDirectory(directory)
}
