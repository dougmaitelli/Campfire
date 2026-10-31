package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

type pruneDocker struct {
	calls  [][]string
	image  string
	shared string
	fail   bool
}

func (d *pruneDocker) Run(_ context.Context, _ io.Reader, args ...string) (string, error) {
	d.calls = append(d.calls, slices.Clone(args))
	if d.fail {
		return "", errors.New("Docker unavailable")
	}

	switch strings.Join(args, " ") {
	case "container ls --all --filter label=campfire.server --format {{json .}}":
		return `{"ID":"game","Names":"campfire-existing","State":"exited"}
{"ID":"installer","Names":"campfire-existing-install","State":"exited"}
{"ID":"active","Names":"campfire-other-install","State":"running"}
{"ID":"deleted","Names":"campfire-deleted","State":"exited"}`, nil
	case "volume ls --quiet --filter label=campfire.server":
		return "unused-campfire-volume\nused-campfire-volume\n", nil
	case "container ls --all --quiet --filter volume=used-campfire-volume":
		return "foreign-container", nil
	case "container ls --all --quiet --filter ancestor=" + d.shared:
		return "foreign-container", nil
	}

	if args[0] == "inspect" {
		return d.shared, nil
	}

	return "", nil
}

func TestPrunePreservesServersSharedImagesAndData(t *testing.T) {
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)

	docker := &pruneDocker{image: "sha256:" + strings.Repeat("a", 64), shared: "sha256:" + strings.Repeat("b", 64)}
	m.Docker = docker

	for _, id := range []string{"existing", "deleted"} {
		state := model.RuntimeStateStopped
		if id == "deleted" {
			state = model.RuntimeStateDeleted
		}

		e := &entry{Images: []string{docker.image, docker.shared}, Status: model.RuntimeStatus{State: state}}
		if err := m.persist(id, e); err != nil {
			t.Fatal(err)
		}
	}

	dataPath := filepath.Join(m.root("existing"), "data", "world")

	backupPath := filepath.Join(m.root("existing"), "backups", "backup.tar.gz")
	for _, path := range []string{dataPath, backupPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte("preserved"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	result, err := m.Prune(t.Context())
	if err != nil || result != (model.PruneResult{Containers: 2, Images: 1, Volumes: 1}) {
		t.Fatalf("cleanup result: %+v, %v", result, err)
	}

	for _, call := range docker.calls {
		joined := strings.Join(call, " ")
		if slices.Contains(call, "--force") || strings.Contains(joined, "system prune") || joined == "container rm game" || joined == "container rm active" || joined == "image rm --no-prune "+docker.shared || joined == "volume rm used-campfire-volume" {
			t.Fatal("unsafe cleanup command", call)
		}
	}

	for _, path := range []string{dataPath, backupPath} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "preserved" {
			t.Fatal("cleanup changed game data or backups", path, err)
		}
	}
}

func TestPruneRejectsActiveWorkAndFailsClosed(t *testing.T) {
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)

	docker := &pruneDocker{}
	m.Docker = docker
	m.maintenance.RLock()
	_, err := m.Prune(t.Context())
	m.maintenance.RUnlock()

	if err == nil || len(docker.calls) != 0 {
		t.Fatal("cleanup ran during active work", err)
	}

	m.maintenance.Lock()
	_, err = m.Execute(t.Context(), runtimeFixture(), model.RuntimeRequest{ServerID: "test-server", Action: "install", OperationID: "install"})
	m.maintenance.Unlock()

	if err == nil {
		t.Fatal("installation started during cleanup")
	}

	docker.fail = true
	if _, err := m.Prune(t.Context()); err == nil || len(docker.calls) != 1 {
		t.Fatal("cleanup continued after Docker inventory failed", docker.calls, err)
	}
}

func TestImageInventoryRetainsPreviousVersions(t *testing.T) {
	m := newTestManager(t.Context(), t.TempDir())
	t.Cleanup(m.Close)

	e := &entry{Images: []string{"sha256:" + strings.Repeat("a", 64)}}
	m.Docker = &imageInventoryDocker{image: "sha256:" + strings.Repeat("b", 64)}
	m.rememberImage("game", "game:latest", e)
	m.rememberImage("game", "game:latest", e)
	data, err := os.ReadFile(filepath.Join(m.root("game"), "state.json"))

	var saved entry
	if err != nil || json.Unmarshal(data, &saved) != nil || len(saved.Images) != 2 {
		t.Fatal("image provenance was not persisted or was duplicated", string(data), err)
	}
}

type imageInventoryDocker struct{ image string }

func (d *imageInventoryDocker) Run(context.Context, io.Reader, ...string) (string, error) {
	return d.image, nil
}
