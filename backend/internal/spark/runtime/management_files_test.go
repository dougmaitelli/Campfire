package runtime

import (
	"campfire/internal/shared/model"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fileTestManager(t *testing.T) (*Manager, model.Deployment) {
	t.Helper()

	d := runtimeFixture()
	m := newTestManager(context.Background(), t.TempDir())
	m.Docker = &fakeDocker{states: make(map[string]string)}
	m.FileOwner = func(*os.File, string) error { return nil }
	t.Cleanup(m.Close)

	if err := os.MkdirAll(filepath.Join(m.root(d.ServerID), "data"), 0700); err != nil {
		t.Fatal(err)
	}

	return m, d
}

func TestManagedFileLifecycleAndRevisions(t *testing.T) {
	m, d := fileTestManager(t)
	m.Docker = &fakeDocker{states: map[string]string{container(d.ServerID): "running"}}

	call := func(q model.FileRequest) model.FileResult {
		t.Helper()

		q.ServerID = d.ServerID

		result, err := m.Files(context.Background(), d, q)
		if err != nil {
			t.Fatal(err)
		}

		return result
	}
	call(model.FileRequest{Operation: "mkdir", Path: "config"})

	binary := []byte{0, 1, 2, 255, 10}
	saved := call(model.FileRequest{Operation: "write", Path: "config/data.bin", Data: binary, Revision: "absent"})

	read := call(model.FileRequest{Operation: "read", Path: "config/data.bin"})
	if string(read.Data) != string(binary) || saved.Revision != read.Revision {
		t.Fatal("binary transfer or revision mismatch")
	}

	result := call(model.FileRequest{Operation: "list", Path: "."})
	if len(result.Entries) != 1 || result.Entries[0].Kind != "directory" {
		t.Fatal("directory listing mismatch")
	}

	updated := call(model.FileRequest{Operation: "write", Path: "config/data.bin", Data: []byte("changed"), Revision: read.Revision})
	if updated.Revision == read.Revision {
		t.Fatal("revision did not change")
	}

	_, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "write", Path: "config/data.bin", Data: []byte("stale"), Revision: read.Revision})

	var failure *model.FileError
	if !errors.As(err, &failure) || failure.Code != "file_conflict" {
		t.Fatalf("stale write: %v", err)
	}

	call(model.FileRequest{Operation: "rename", Path: "config/data.bin", Destination: "config/renamed.bin"})
	call(model.FileRequest{Operation: "delete", Path: "config/renamed.bin"})
	call(model.FileRequest{Operation: "delete", Path: "config"})
	call(model.FileRequest{Operation: "write", Path: "empty.txt", Data: []byte{}, Revision: "absent"})

	if result := call(model.FileRequest{Operation: "read", Path: "empty.txt"}); len(result.Data) != 0 {
		t.Fatal("empty file changed")
	}
}

func TestManagedFileContainmentAndLimits(t *testing.T) {
	m, d := fileTestManager(t)
	root := filepath.Join(m.root(d.ServerID), "data")

	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	for _, q := range []model.FileRequest{{Operation: "read", Path: "../state.json"}, {Operation: "write", Path: "/tmp/outside", Revision: "absent"}, {Operation: "read", Path: "link"}, {Operation: "delete", Path: "."}, {Operation: "mkdir", Path: "bad\\name"}, {Operation: "rename", Path: "link", Destination: "moved"}, {Operation: "list", Path: "../"}} {
		q.ServerID = d.ServerID
		if _, err := m.Files(context.Background(), d, q); err == nil {
			t.Errorf("unsafe request accepted: %s %s", q.Operation, q.Path)
		}
	}

	if data, err := os.ReadFile(outside); err != nil || string(data) != "secret" {
		t.Fatal("outside file changed")
	}

	data := make([]byte, model.FileLimit+1)
	if err := os.WriteFile(filepath.Join(root, "large"), data, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "read", Path: "large"})

	var failure *model.FileError
	if !errors.As(err, &failure) || failure.Code != "file_limit" {
		t.Fatalf("oversized read: %v", err)
	}

	d.Configuration.DiskMB = 1

	_, err = m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "write", Path: "new", Data: []byte("x"), Revision: "absent"})
	if !errors.As(err, &failure) || failure.Code != "file_limit" {
		t.Fatalf("disk capacity: %v", err)
	}
}

func TestManagedFileRunningAndBusyGuards(t *testing.T) {
	m, d := fileTestManager(t)

	docker, ok := m.Docker.(*fakeDocker)
	if !ok {
		t.Fatal("missing test Docker executor")
	}

	docker.states[container(d.ServerID)] = "running"
	if err := os.WriteFile(filepath.Join(m.root(d.ServerID), "data", "world"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "read", Path: "world"}); err != nil {
		t.Fatal("running read failed", err)
	}

	_, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "delete", Path: "world"})
	if err != nil {
		t.Fatalf("running mutation: %v", err)
	}

	entry, err := m.load(d.ServerID)
	if err != nil {
		t.Fatal(err)
	}

	entry.Status.Busy = true

	if _, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "list", Path: "."}); err == nil || !strings.Contains(err.Error(), "operation") {
		t.Fatal("busy runtime allowed file access")
	}
}
