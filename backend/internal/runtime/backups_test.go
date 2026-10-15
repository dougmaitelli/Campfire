package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/model"
)

func awaitBackup(t *testing.T, m *Manager, d model.Deployment, q model.RuntimeRequest) model.RuntimeStatus {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	q.ServerID = d.ServerID

	status, err := m.Execute(ctx, d, q)
	if err != nil {
		t.Fatal(err)
	}

	for status.Busy {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}

		status, err = m.Execute(ctx, d, model.RuntimeRequest{ServerID: d.ServerID, Action: "status"})
		if err != nil {
			t.Fatal(err)
		}
	}

	return status
}

func TestBackupRoundTripChunksAndDeletion(t *testing.T) {
	m, d := fileTestManager(t)
	m.Ownership = func(string, string) error { return nil }

	if status := awaitOperation(t, m, d, "install", "install"); status.Error != "" {
		t.Fatal(status.Error)
	}

	volume := filepath.Join(m.root(d.ServerID), "data")

	original := bytes.Repeat([]byte("saved world\x00\xff"), 1024)
	if err := os.WriteFile(filepath.Join(volume, "world"), original, 0640); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(volume, "config"), 0750); err != nil {
		t.Fatal(err)
	}

	if status := awaitOperation(t, m, d, "backup", "snapshot"); status.Error != "" {
		t.Fatal(status.Error)
	}

	// Duplicate submission must not publish a second archive.
	awaitOperation(t, m, d, "backup", "snapshot")

	listed, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "backup.list"})
	if err != nil || len(listed.Entries) != 1 || listed.Entries[0].Name != "snapshot.tar.gz" {
		t.Fatalf("backup list: %+v %v", listed, err)
	}

	chunk, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "backup.read", Path: "snapshot.tar.gz"})
	if err != nil || chunk.Size != int64(len(chunk.Data)) {
		t.Fatalf("backup read: %v", err)
	}

	if err := extractBackup(context.Background(), bytes.NewReader(chunk.Data), t.TempDir(), backupBytes); err != nil {
		t.Fatal("download is not a valid archive", err)
	}

	if err := os.WriteFile(filepath.Join(volume, "world"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(volume, "extra"), nil, 0600); err != nil {
		t.Fatal(err)
	}

	status := awaitBackup(t, m, d, model.RuntimeRequest{Action: "restore", OperationID: "restore", Content: "snapshot.tar.gz"})
	if status.Error != "" || status.State != "stopped" {
		t.Fatalf("restore: %+v", status)
	}

	data, err := os.ReadFile(filepath.Join(volume, "world"))
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("world was not restored", err)
	}

	if _, err := os.Stat(filepath.Join(volume, "extra")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("restore retained a file absent from the snapshot")
	}

	for name, mode := range map[string]os.FileMode{"world": 0640, "config": 0750} {
		info, err := os.Stat(filepath.Join(volume, name))
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("restored permissions for %s: %v", name, err)
		}
	}

	// Archive chunks must remain bounded even when the archive exceeds one frame.
	large := bytes.Repeat([]byte{42}, model.FileLimit+37)
	if err := os.WriteFile(filepath.Join(m.root(d.ServerID), "backups", "large.tar.gz"), large, 0600); err != nil {
		t.Fatal(err)
	}

	first, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "backup.read", Path: "large.tar.gz"})
	if err != nil || len(first.Data) != model.FileLimit || first.Size != int64(len(large)) {
		t.Fatal("first download chunk", err)
	}

	last, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "backup.read", Path: "large.tar.gz", Offset: model.FileLimit})
	if err != nil || len(last.Data) != 37 {
		t.Fatal("last download chunk", err)
	}

	if status := awaitOperation(t, m, d, "delete", "delete"); status.Error != "" {
		t.Fatal(status.Error)
	}

	if _, err := os.Stat(filepath.Join(m.root(d.ServerID), "backups")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("server deletion retained backups")
	}
}

func testArchive(t *testing.T, headers []*tar.Header) []byte {
	t.Helper()

	var buffer bytes.Buffer

	compressed := gzip.NewWriter(&buffer)

	archive := tar.NewWriter(compressed)
	for _, header := range headers {
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}

		if header.Typeflag == tar.TypeReg {
			if _, err := io.CopyN(archive, bytes.NewReader(bytes.Repeat([]byte("x"), int(header.Size))), header.Size); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}

	return buffer.Bytes()
}

func TestBackupRejectsUnsafeArchivesAndPreservesWorld(t *testing.T) {
	valid := testArchive(t, []*tar.Header{{Name: "world", Typeflag: tar.TypeReg, Size: 4, Mode: 0600}})
	corrupt := append([]byte(nil), valid...)
	corrupt[len(corrupt)-8] ^= 0xff

	cases := map[string][]byte{
		"checksum":  corrupt,
		"truncated": valid[:len(valid)-5],
		"traversal": testArchive(t, []*tar.Header{{Name: "../outside", Typeflag: tar.TypeReg}}),
		"absolute":  testArchive(t, []*tar.Header{{Name: "/outside", Typeflag: tar.TypeReg}}),
		"symlink":   testArchive(t, []*tar.Header{{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside"}}),
		"hardlink":  testArchive(t, []*tar.Header{{Name: "link", Typeflag: tar.TypeLink, Linkname: "world"}}),
		"fifo":      testArchive(t, []*tar.Header{{Name: "pipe", Typeflag: tar.TypeFifo}}),
		"duplicate": testArchive(t, []*tar.Header{{Name: "world", Typeflag: tar.TypeReg}, {Name: "world", Typeflag: tar.TypeReg}}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			m, d := fileTestManager(t)
			m.Ownership = func(string, string) error { return nil }
			awaitOperation(t, m, d, "install", "install")

			world := filepath.Join(m.root(d.ServerID), "data", "world")
			if err := os.WriteFile(world, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}

			directory := filepath.Join(m.root(d.ServerID), "backups")
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(filepath.Join(directory, "bad.tar.gz"), data, 0600); err != nil {
				t.Fatal(err)
			}

			status := awaitBackup(t, m, d, model.RuntimeRequest{Action: "restore", OperationID: "restore", Content: "bad.tar.gz"})
			if status.Error == "" {
				t.Fatal("unsafe archive accepted")
			}

			current, err := os.ReadFile(world)
			if err != nil || string(current) != "original" {
				t.Fatal("failed restore changed the world", err)
			}
		})
	}

	if err := extractBackup(context.Background(), bytes.NewReader(valid), t.TempDir(), 3); err == nil {
		t.Fatal("expanded archive size limit ignored")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := extractBackup(ctx, bytes.NewReader(valid), t.TempDir(), backupBytes); err == nil {
		t.Fatal("cancelled extraction succeeded")
	}
}

func TestBackupGuardsAndOwnershipFailure(t *testing.T) {
	m, d := fileTestManager(t)
	m.Ownership = func(string, string) error { return nil }
	awaitOperation(t, m, d, "install", "install")
	awaitOperation(t, m, d, "backup", "snapshot")
	awaitOperation(t, m, d, "start", "start")

	for _, action := range []string{"backup", "restore"} {
		_, err := m.Execute(context.Background(), d, model.RuntimeRequest{ServerID: d.ServerID, Action: action, OperationID: "blocked", Content: "snapshot.tar.gz"})
		if err == nil {
			t.Fatal("running backup operation accepted", action)
		}
	}

	awaitOperation(t, m, d, "stop", "stop")

	world := filepath.Join(m.root(d.ServerID), "data", "current")
	if err := os.WriteFile(world, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}

	m.Ownership = func(string, string) error { return errors.New("ownership failed") }
	if status := awaitBackup(t, m, d, model.RuntimeRequest{Action: "restore", OperationID: "restore", Content: "snapshot.tar.gz"}); status.Error == "" {
		t.Fatal("ownership failure ignored")
	}

	if data, err := os.ReadFile(world); err != nil || string(data) != "preserve" {
		t.Fatal("ownership failure changed world", err)
	}

	m.Ownership = func(string, string) error { return nil }

	m.Docker = backupRemoveFailure{m.Docker}
	if status := awaitBackup(t, m, d, model.RuntimeRequest{Action: "restore", OperationID: "remove-failure", Content: "snapshot.tar.gz"}); status.Error == "" {
		t.Fatal("container removal failure ignored")
	}

	if data, err := os.ReadFile(world); err != nil || string(data) != "preserve" {
		t.Fatal("container removal failure changed world", err)
	}

	for _, name := range []string{"../snapshot.tar.gz", "/snapshot.tar.gz", "snapshot", "a\\b.tar.gz"} {
		if _, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "backup.read", Path: name}); err == nil {
			t.Fatal("unsafe backup name accepted", name)
		}
	}

	if err := os.Symlink(world, filepath.Join(m.root(d.ServerID), "backups", "link.tar.gz")); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "backup.read", Path: "link.tar.gz"}); err == nil {
		t.Fatal("backup symlink followed")
	}

	if err := os.Symlink(world, filepath.Join(m.root(d.ServerID), "data", "link")); err != nil {
		t.Fatal(err)
	}

	if status := awaitOperation(t, m, d, "backup", "unsafe"); status.Error == "" {
		t.Fatal("backup of a symlink succeeded")
	}

	if _, err := os.Stat(filepath.Join(m.root(d.ServerID), "backups", "unsafe.tar.gz")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed backup was published")
	}
}

type backupRemoveFailure struct{ Executor }

func (d backupRemoveFailure) Run(ctx context.Context, input io.Reader, args ...string) (string, error) {
	if args[0] == "rm" {
		return "", errors.New("container removal failed")
	}

	return d.Executor.Run(ctx, input, args...)
}

func TestInterruptedRestoreBlocksStartAfterRestart(t *testing.T) {
	m, d := fileTestManager(t)
	m.Ownership = func(string, string) error { return nil }
	awaitOperation(t, m, d, "install", "install")

	// Simulate a process exit between preserving the original and publishing the stage.
	if err := os.Rename(filepath.Join(m.root(d.ServerID), "data"), filepath.Join(m.root(d.ServerID), "restore-previous")); err != nil {
		t.Fatal(err)
	}

	m.Close()

	restarted := New(context.Background(), m.Directory)
	restarted.Docker = m.Docker
	restarted.Ownership = m.Ownership
	t.Cleanup(restarted.Close)

	status := awaitOperation(t, restarted, d, "start", "blocked-start")
	if status.Error == "" {
		t.Fatal("interrupted restore started a server with missing game data")
	}

	if _, err := os.Stat(filepath.Join(m.root(d.ServerID), "restore-previous")); err != nil {
		t.Fatal("original data was lost", err)
	}

	if _, err := os.Stat(filepath.Join(m.root(d.ServerID), "data")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("start created replacement game data before recovery")
	}

	if status := awaitOperation(t, restarted, d, "delete", "delete"); status.Error != "" {
		t.Fatal(status.Error)
	}

	if _, err := os.Stat(filepath.Join(m.root(d.ServerID), "restore-previous")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("explicit server deletion retained previous game data")
	}
}
