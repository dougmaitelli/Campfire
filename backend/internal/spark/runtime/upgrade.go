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

// reinstall keeps existing files for the legacy update operation.
func (m *Manager) reinstall(ctx context.Context, d model.Deployment, e *entry, pull bool) error {
	return m.reinstallMode(ctx, d, e, pull, false)
}

// A failed installer leaves the server uninstalled until a successful retry.
// Clean installs clear only game files; local backups remain available.
func (m *Manager) reinstallMode(ctx context.Context, d model.Deployment, e *entry, pull, clean bool) error {
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

	if clean {
		m.progress(d.ServerID, e, "Removing game files for clean install")

		output, err := m.Docker.Run(ctx, nil, "rm", "--force", container(d.ServerID)+"-install")
		if err != nil && !bytes.Contains(bytes.ToLower([]byte(output)), []byte("no such")) {
			return err
		}

		volume, err := m.volume(d.ServerID)
		if err != nil {
			return err
		}
		// Keep the volume directory itself so its quota assignment is preserved.
		root, err := os.OpenRoot(volume)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		if err == nil {
			defer func() { _ = root.Close() }()

			dir, err := root.Open(".")
			if err != nil {
				return err
			}

			names, readErr := dir.Readdirnames(-1)
			_ = dir.Close()

			if readErr != nil {
				return readErr
			}

			for _, name := range names {
				if err := root.RemoveAll(name); err != nil {
					return err
				}
			}
		}
	}

	return m.install(ctx, d, e)
}
