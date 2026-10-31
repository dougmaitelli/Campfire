package runtime

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"campfire/internal/shared/model"
)

type transferManifest struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	ServerID  string `json:"server_id"`
	EmberID   string `json:"ember_id"`
	Installed bool   `json:"installed"`
}

var errTransferChecksum = errors.New("incoming transfer checksum mismatch")

type transferWriter struct {
	io.Writer
	written int64
}

func (w *transferWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > backupBytes-w.written {
		return 0, errors.New("transfer archive exceeds 100 GiB")
	}

	n, err := w.Writer.Write(data)
	w.written += int64(n)

	return n, err
}

func (m *Manager) exportTransfer(ctx context.Context, d model.Deployment, t *localTransfer) error {
	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return err
	}

	file, err := os.CreateTemp(m.root(d.ServerID), ".transfer-build-"+t.ID+"-")
	if err != nil {
		return err
	}

	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()

	hash := sha256.New()
	output := &transferWriter{Writer: io.MultiWriter(file, hash)}
	compressed := gzip.NewWriter(output)
	archive := tar.NewWriter(compressed)

	manifest, err := json.Marshal(transferManifest{Version: 1, ID: t.ID, ServerID: d.ServerID, EmberID: d.Configuration.EmberID, Installed: t.Installed})
	if err != nil {
		return err
	}

	if err := archive.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(manifest)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}

	if _, err := archive.Write(manifest); err != nil {
		return err
	}

	var total int64

	count := 0

	for _, name := range []string{"data", "backups"} {
		if err := archive.WriteHeader(&tar.Header{Name: name + "/", Mode: 0700, Typeflag: tar.TypeDir}); err != nil {
			return err
		}

		root, err := os.OpenRoot(filepath.Join(m.root(d.ServerID), name))
		if errors.Is(err, os.ErrNotExist) && (name == "backups" || !t.Installed) {
			continue
		}

		if err != nil {
			return err
		}

		err = transferTree(ctx, root, name, archive, &total, &count)
		_ = root.Close()

		if err != nil {
			return err
		}
	}

	if err := archive.Close(); err != nil {
		return err
	}

	if err := compressed.Close(); err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return err
	}

	if err := os.Rename(file.Name(), m.transferArchive(d.ServerID, t.ID)); err != nil {
		return err
	}

	if err := syncBackupDirectory(m.root(d.ServerID)); err != nil {
		return err
	}

	t.Size, t.Digest, t.State = output.written, hex.EncodeToString(hash.Sum(nil)), model.TransferStateReady

	return nil
}

func transferTree(ctx context.Context, root *os.Root, prefix string, archive *tar.Writer, total *int64, count *int) error {
	return fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if name == "." {
			return nil
		}

		if prefix == "backups" && (entry.IsDir() || !backupName(name)) {
			if entry.IsDir() {
				return fs.SkipDir
			}

			return nil
		}

		*count++
		if *count > backupEntries || !filePath(name, false) {
			return errors.New("transfer tree exceeds entry or path limits")
		}

		if err := containedPath(root, name, false); err != nil {
			return err
		}

		file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}

		defer func() { _ = file.Close() }()

		info, err := file.Stat()
		if err != nil {
			return err
		}

		if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("transfer cannot contain links or special files")
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		header.Name, header.Mode = prefix+"/"+name, header.Mode&0777
		header.Uid, header.Gid, header.Uname, header.Gname = 0, 0, "", ""

		*total += header.Size
		if *total > backupBytes {
			return errors.New("transfer contents exceed 100 GiB")
		}

		if err := archive.WriteHeader(header); err != nil {
			return err
		}

		if info.Mode().IsRegular() {
			_, err = io.CopyN(archive, backupReader{ctx, file}, info.Size())
		}

		return err
	})
}

func (m *Manager) importTransfer(ctx context.Context, d model.Deployment, t *localTransfer) error {
	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return err
	}

	file, err := os.Open(m.transferArchive(d.ServerID, t.ID))
	if err != nil {
		return err
	}

	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != t.Size {
		return errors.New("incoming transfer size mismatch")
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, backupReader{ctx, file}); err != nil {
		return err
	}

	if hex.EncodeToString(hash.Sum(nil)) != t.Digest {
		return errTransferChecksum
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	entries, err := os.ReadDir(m.root(d.ServerID))
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".transfer-stage-"+t.ID+"-") {
			if err := os.RemoveAll(filepath.Join(m.root(d.ServerID), entry.Name())); err != nil {
				return err
			}
		}
	}

	stage, err := os.MkdirTemp(m.root(d.ServerID), ".transfer-stage-"+t.ID+"-")
	if err != nil {
		return err
	}

	defer func() { _ = os.RemoveAll(stage) }()

	for _, name := range []string{"data", "backups"} {
		if err := os.Mkdir(filepath.Join(stage, name), 0700); err != nil {
			return err
		}
	}

	data := filepath.Join(stage, "data")
	if err := m.prepareQuota(data, d.Configuration.DiskMB); err != nil {
		return err
	}

	installed, err := extractTransfer(ctx, file, stage, d, t.ID)
	if err != nil {
		return err
	}

	if installed {
		if err := configure(data, d); err != nil {
			return err
		}
	}

	if err := m.Ownership(data, d.Ember.User); err != nil {
		return err
	}

	if err := m.prepareQuota(data, d.Configuration.DiskMB); err != nil {
		return err
	}

	if err := filepath.WalkDir(stage, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if entry.IsDir() {
			return syncBackupDirectory(name)
		}

		return nil
	}); err != nil {
		return err
	}

	if err := m.backupStopped(ctx, d.ServerID); err != nil {
		return err
	}

	state, err := m.inspect(ctx, d.ServerID)
	if err != nil {
		return err
	}

	if state != "missing" {
		if _, err := m.Docker.Run(ctx, nil, "rm", container(d.ServerID)); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	for _, name := range []string{"data", "backups"} {
		if err := os.RemoveAll(filepath.Join(m.root(d.ServerID), name)); err != nil {
			return err
		}

		if err := os.Rename(filepath.Join(stage, name), filepath.Join(m.root(d.ServerID), name)); err != nil {
			return err
		}
	}

	if err := syncBackupDirectory(m.root(d.ServerID)); err != nil {
		return err
	}

	t.Installed, t.State = installed, model.TransferStateImported

	return nil
}

func extractTransfer(ctx context.Context, input io.Reader, directory string, d model.Deployment, transferID string) (bool, error) {
	compressed, err := gzip.NewReader(backupReader{ctx, input})
	if err != nil {
		return false, err
	}
	defer func() { _ = compressed.Close() }()

	archive := tar.NewReader(backupReader{ctx, compressed})

	root, err := os.OpenRoot(directory)
	if err != nil {
		return false, err
	}

	defer func() { _ = root.Close() }()

	first, err := archive.Next()
	if err != nil || first.Name != "manifest.json" || first.Typeflag != tar.TypeReg || first.Size < 1 || first.Size > 4096 {
		return false, errors.New("invalid transfer manifest")
	}

	var manifest transferManifest
	if err := json.NewDecoder(archive).Decode(&manifest); err != nil {
		return false, err
	}

	if manifest.Version != 1 || manifest.ID != transferID || manifest.ServerID != d.ServerID || manifest.EmberID != d.Configuration.EmberID {
		return false, errors.New("transfer manifest identity mismatch")
	}

	seen := make(map[string]bool)
	directories := make(map[string]fs.FileMode)

	var total, dataBytes int64

	for count := 0; ; count++ {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return false, err
		}

		name := strings.TrimSuffix(header.Name, "/")
		isData := strings.HasPrefix(name, "data/")
		isBackup := strings.HasPrefix(name, "backups/") && backupName(strings.TrimPrefix(name, "backups/"))

		isRoot := (name == "data" || name == "backups") && header.Typeflag == tar.TypeDir
		if count > backupEntries+2 || seen[name] || !filePath(name, false) || (!isData && !isBackup && !isRoot) || (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir) || header.Size < 0 || header.Size > backupBytes-total {
			return false, errors.New("invalid transfer archive entry")
		}

		seen[name] = true

		if isBackup && header.Typeflag != tar.TypeReg {
			return false, errors.New("completed backups must be regular files")
		}

		total += header.Size
		if isData {
			dataBytes += header.Size
		}

		if d.Configuration.DiskMB > 0 && dataBytes > d.Configuration.DiskMB<<20 {
			return false, errors.New("incoming game data exceeds allocation")
		}

		if header.Typeflag == tar.TypeDir {
			if header.Size != 0 {
				return false, errors.New("invalid transfer directory")
			}

			if err := root.MkdirAll(name, 0700); err != nil {
				return false, err
			}

			directories[name] = fs.FileMode(header.Mode) & 0777

			continue
		}

		if err := root.MkdirAll(path.Dir(name), 0700); err != nil {
			return false, err
		}

		if err := writeTransferEntry(root, name, archive, header); err != nil {
			return false, err
		}
	}

	paths := make([]string, 0, len(directories))
	for name := range directories {
		paths = append(paths, name)
	}

	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })

	for _, name := range paths {
		if err := root.Chmod(name, directories[name]); err != nil {
			return false, err
		}
	}

	return manifest.Installed, nil
}

func writeTransferEntry(root *os.Root, name string, archive io.Reader, header *tar.Header) error {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if _, err := io.CopyN(file, archive, header.Size); err != nil {
		return err
	}

	if err := file.Chmod(fs.FileMode(header.Mode) & 0777); err != nil {
		return err
	}

	return file.Sync()
}
