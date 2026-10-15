package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"campfire/internal/shared/model"
)

// reinstall never removes game data or backups. A failed installer leaves the
// server uninstalled until an explicit successful retry; start cannot bypass it.
func (m *Manager) reinstall(ctx context.Context, d model.Deployment, e *entry, pull bool) error {
	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return err
	}

	if _, err := os.Lstat(filepath.Join(m.root(d.ServerID), "restore-previous")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("previous restore data requires operator recovery before reinstalling")
	}

	output, err := m.Docker.Run(ctx, nil, "rm", container(d.ServerID))
	if err != nil && !bytes.Contains(bytes.ToLower([]byte(output)), []byte("no such")) {
		return err
	}

	e.mu.Lock()
	e.Installed, e.Revision = false, ""
	err = m.persist(d.ServerID, e)
	e.mu.Unlock()

	if err != nil {
		return err
	}

	if pull {
		m.progress(d.ServerID, e, "Pulling current images")

		images := []string{d.Configuration.Image}
		if d.Ember.Installer != nil && d.Ember.Installer.Image != d.Configuration.Image {
			images = append(images, d.Ember.Installer.Image)
		}

		for _, image := range images {
			if _, err := m.Docker.Run(ctx, nil, "pull", image); err != nil {
				return fmt.Errorf("pull server image: %w", err)
			}
		}
	}

	return m.install(ctx, d, e)
}
