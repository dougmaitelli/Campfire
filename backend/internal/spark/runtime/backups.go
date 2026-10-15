package runtime

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"campfire/internal/shared/model"
)

const backupBytes int64 = 100 << 30
const backupEntries = 100000

func backupName(name string) bool {
	return strings.HasSuffix(name, ".tar.gz") && safeID.MatchString(strings.TrimSuffix(name, ".tar.gz"))
}

func (m *Manager) backupStopped(ctx context.Context, id string) error {
	state, err := m.inspect(ctx, id)
	if err != nil {
		return errors.New("could not verify server state")
	}

	if state != "missing" && state != "exited" && state != "created" {
		return errors.New("stop the server before creating or restoring a backup")
	}

	return nil
}

// backupFiles lists immutable archives and reads bounded chunks over the file protocol.
func (m *Manager) backupFiles(ctx context.Context, d model.Deployment, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: q.ServerID, Path: q.Path}
	if !safeID.MatchString(q.ServerID) || q.ServerID != d.ServerID || (q.Operation != "backup.list" && q.Operation != "backup.read") || q.Offset < 0 {
		return result, fileFailure("invalid_file", "Invalid backup request.")
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	e, err := m.load(q.ServerID)
	if err != nil {
		return result, fileFailure("file_io", "Backup storage unavailable.")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Status.Busy {
		return result, fileFailure("file_busy", "Wait for the current server operation to finish.")
	}

	root, err := os.OpenRoot(filepath.Join(m.root(q.ServerID), "backups"))
	if errors.Is(err, os.ErrNotExist) && q.Operation == "backup.list" {
		return result, nil
	}

	if err != nil {
		return result, fileFailure("file_not_found", "Backup not found.")
	}

	defer func() { _ = root.Close() }()

	if q.Operation == "backup.list" {
		dir, err := root.Open(".")
		if err != nil {
			return result, fileFailure("file_io", "Could not list backups.")
		}
		defer func() { _ = dir.Close() }()

		entries, err := dir.ReadDir(1002)
		if err != nil && !errors.Is(err, io.EOF) {
			return result, fileFailure("file_io", "Could not list backups.")
		}

		if len(entries) > 1000 {
			return result, fileFailure("file_limit", "Backup storage exceeds the 1000-entry limit.")
		}

		for _, item := range entries {
			info, err := item.Info()
			if err != nil {
				return result, fileFailure("file_io", "Could not inspect backup.")
			}

			if backupName(item.Name()) && info.Mode().IsRegular() {
				result.Entries = append(result.Entries, model.FileEntry{Name: item.Name(), Kind: "file", Size: info.Size(), ModifiedAt: info.ModTime().UTC()})
			}
		}

		sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].ModifiedAt.After(result.Entries[j].ModifiedAt) })

		return result, nil
	}

	if !backupName(q.Path) {
		return result, fileFailure("invalid_file", "Invalid backup name.")
	}

	file, err := root.OpenFile(q.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return result, fileFailure("file_not_found", "Backup not found.")
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || q.Offset > info.Size() {
		return result, fileFailure("invalid_file", "Invalid backup or offset.")
	}

	result.Size = info.Size()

	result.Data = make([]byte, min(int64(model.FileLimit), info.Size()-q.Offset))
	if n, err := file.ReadAt(result.Data, q.Offset); err != nil || n != len(result.Data) {
		return result, fileFailure("file_io", "Could not read backup.")
	}

	return result, nil
}

type backupReader struct {
	ctx context.Context
	io.Reader
}

func (r backupReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	return r.Reader.Read(p)
}

func (m *Manager) createBackup(ctx context.Context, d model.Deployment, id string, e *entry) error {
	m.progress(d.ServerID, e, "Creating backup")

	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return err
	}

	directory := filepath.Join(m.root(d.ServerID), "backups")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return errors.New("could not prepare backup storage")
	}

	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) >= 1000 {
		return errors.New("backup storage unavailable or 1000-backup limit reached")
	}

	file, err := os.CreateTemp(directory, ".partial-")
	if err != nil {
		return errors.New("could not prepare backup")
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()

	volume, err := m.volume(d.ServerID)
	if err != nil {
		return err
	}

	root, err := os.OpenRoot(volume)
	if err != nil {
		return errors.New("game data unavailable")
	}
	defer func() { _ = root.Close() }()

	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)

	var total int64

	count := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if name == "." {
			return nil
		}

		count++
		if count > backupEntries || !filePath(name, false) {
			return errors.New("backup contains too many entries or invalid paths")
		}

		if err := containedPath(root, name, false); err != nil {
			return err
		}

		input, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { _ = input.Close() }()

		info, err := input.Stat()
		if err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("backups cannot contain links or special files")
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		header.Name = name
		header.Uid, header.Gid = 0, 0
		header.Uname, header.Gname = "", ""
		header.Mode &= 0777

		total += header.Size
		if total > backupBytes {
			return errors.New("backup exceeds 100 GiB")
		}

		if err := archive.WriteHeader(header); err != nil {
			return err
		}

		if info.Mode().IsRegular() {
			_, err = io.CopyN(archive, backupReader{ctx, input}, info.Size())
		}

		return err
	})
	archiveErr := archive.Close()
	gzipErr := compressed.Close()
	syncErr := file.Sync()

	closeErr := file.Close()
	if err != nil || archiveErr != nil || gzipErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("backup failed; check disk space, data paths, links and the 100 GiB/100000-entry limits")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.Rename(file.Name(), filepath.Join(directory, id+".tar.gz")); err != nil {
		return errors.New("could not publish backup")
	}

	if err := syncBackupDirectory(directory); err != nil {
		return errors.New("backup created but storage synchronization failed")
	}

	return nil
}

func syncBackupDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	return file.Sync()
}

func extractBackup(ctx context.Context, input io.Reader, directory string, limit int64) error {
	compressed, err := gzip.NewReader(backupReader{ctx, input})
	if err != nil {
		return err
	}
	defer func() { _ = compressed.Close() }()

	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	archive := tar.NewReader(compressed)
	seen := make(map[string]bool)

	type directoryMode struct {
		name string
		mode fs.FileMode
	}

	var directories []directoryMode

	var total int64

	for count := 0; ; count++ {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		name := strings.TrimSuffix(header.Name, "/")
		if count >= backupEntries || !filePath(name, false) || seen[name] || (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir) || header.Size < 0 || header.Size > limit-total {
			return errors.New("invalid backup entry or archive limit exceeded")
		}

		seen[name] = true
		total += header.Size

		if err := ctx.Err(); err != nil {
			return err
		}

		if err := root.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return err
		}

		if header.Typeflag == tar.TypeDir {
			if err := root.MkdirAll(name, 0700); err != nil {
				return err
			}

			directories = append(directories, directoryMode{name: name, mode: fs.FileMode(header.Mode) & 0777})

			continue
		}

		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}

		_, copyErr := io.CopyN(file, backupReader{ctx, archive}, header.Size)
		modeErr := file.Chmod(fs.FileMode(header.Mode) & 0777)
		syncErr := file.Sync()

		closeErr := file.Close()
		if copyErr != nil || modeErr != nil || syncErr != nil || closeErr != nil {
			return errors.New("could not extract backup file")
		}
	}

	// Consume the gzip footer to verify its checksum; reject excessive trailing data.
	n, err := io.Copy(io.Discard, io.LimitReader(compressed, 1<<20))
	if err != nil || n == 1<<20 {
		return errors.New("backup checksum or trailing data is invalid")
	}

	// Restore directory permissions after their children have been extracted.
	sort.Slice(directories, func(i, j int) bool { return len(directories[i].name) > len(directories[j].name) })

	for _, item := range directories {
		if err := root.Chmod(item.name, item.mode); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) restoreBackup(ctx context.Context, d model.Deployment, name string, e *entry) error {
	m.progress(d.ServerID, e, "Validating and preparing backup")

	if !backupName(name) {
		return errors.New("invalid backup name")
	}

	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return err
	}

	root, err := os.OpenRoot(filepath.Join(m.root(d.ServerID), "backups"))
	if err != nil {
		return errors.New("backup storage unavailable")
	}
	defer func() { _ = root.Close() }()

	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return errors.New("backup not found")
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("invalid backup file")
	}

	stage, err := os.MkdirTemp(m.root(d.ServerID), ".restore-")
	if err != nil {
		return errors.New("could not prepare restoration")
	}
	defer func() { _ = os.RemoveAll(stage) }()

	limit := min(backupBytes, d.Configuration.DiskMB<<20)
	if err := extractBackup(ctx, file, stage, limit); err != nil {
		return errors.New("backup validation failed; original game data was preserved")
	}

	if err := m.Ownership(stage, d.Ember.User); err != nil {
		return errors.New("could not assign restored game ownership; original game data was preserved")
	}

	if err := filepath.WalkDir(stage, func(name string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if item.IsDir() {
			return syncBackupDirectory(name)
		}

		return nil
	}); err != nil {
		return errors.New("could not synchronize staged restoration; original game data was preserved")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return err
	}

	previous := filepath.Join(m.root(d.ServerID), "restore-previous")
	if _, err := os.Lstat(previous); !errors.Is(err, os.ErrNotExist) {
		return errors.New("previous restore data requires operator recovery before another restore")
	}

	state, err := m.inspect(ctx, d.ServerID)
	if err != nil {
		return errors.New("could not inspect stopped container")
	}

	if state != "missing" {
		if _, err := m.Docker.Run(ctx, nil, "rm", container(d.ServerID)); err != nil {
			return errors.New("could not remove stopped container; original game data was preserved")
		}
	}

	m.progress(d.ServerID, e, "Replacing game data")

	volume, err := m.volume(d.ServerID)
	if err != nil {
		return err
	}

	if err := os.Rename(volume, previous); err != nil {
		return errors.New("could not preserve original game data")
	}

	if err := syncBackupDirectory(m.root(d.ServerID)); err != nil {
		_ = os.Rename(previous, volume)
		return errors.New("could not synchronize preserved game data; inspect restore-previous before retrying")
	}

	if err := os.Rename(stage, volume); err != nil {
		if rollbackErr := os.Rename(previous, volume); rollbackErr != nil {
			return errors.New("restore failed; original data is preserved in restore-previous and requires operator recovery")
		}

		return errors.New("restore failed; original game data was preserved")
	}

	if err := syncBackupDirectory(m.root(d.ServerID)); err != nil {
		return errors.New("restored data synchronization failed; original data remains in restore-previous for operator recovery")
	}

	if err := os.RemoveAll(previous); err != nil {
		return errors.New("game data restored; previous data cleanup requires operator attention")
	}

	return nil
}
