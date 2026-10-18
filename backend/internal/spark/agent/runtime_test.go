package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"campfire/internal/campfire/agents"
	"campfire/internal/shared/model"
)

type wireDocker struct {
	mu           sync.Mutex
	state        string
	console      chan string
	followActive atomic.Int64
}

func (d *wireDocker) Follow(ctx context.Context, _ string, emit func(string) error) error {
	d.followActive.Add(1)
	defer d.followActive.Add(-1)

	if err := emit("recent history\n"); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case line := <-d.console:
			if line == "end" {
				return nil
			}

			if line == "burst" {
				if err := emit(strings.Repeat("\x80", 9000)); err != nil {
					return err
				}

				for range 128 {
					if err := emit(strings.Repeat("🎮", 2049)); err != nil {
						return err
					}
				}

				continue
			}

			if err := emit(line); err != nil {
				return err
			}
		}
	}
}

func testWireConsole(t *testing.T, hub *agents.Hub, id string, docker *wireDocker) {
	t.Helper()

	deployment := deploymentFixture(id, "streaming-console")
	if _, err := hub.Deploy(context.Background(), id, deployment); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for _, action := range []string{"install", "start"} {
		status, err := hub.Action(ctx, id, model.RuntimeRequest{ServerID: deployment.ServerID, Action: action, OperationID: "console-" + action})
		if err != nil {
			t.Fatal(err)
		}

		for status.Busy {
			status, err = hub.Action(ctx, id, model.RuntimeRequest{ServerID: deployment.ServerID, Action: "status"})
			if err != nil {
				t.Fatal(err)
			}
		}

		if status.ErrorCode != "" {
			t.Fatal(status)
		}
	}

	events := make(chan model.ConsoleEvent, 16)

	finished := make(chan error, 1)
	go func() {
		finished <- hub.Console(ctx, id, deployment.ServerID, func(event model.ConsoleEvent) error { events <- event; return nil })
	}()

	select {
	case event := <-events:
		if !event.Ready {
			t.Fatal("stream did not acknowledge", event)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	select {
	case event := <-events:
		if event.Content != "recent history\n" {
			t.Fatal(event)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// A long-lived console must not block ordinary correlated requests.
	if _, err := hub.Deploy(ctx, id, deployment); err != nil {
		t.Fatal("console blocked commands", err)
	}

	select {
	case docker.console <- "new output without polling\n":
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	select {
	case event := <-events:
		if event.Content != "new output without polling\n" {
			t.Fatal(event)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	cancel()

	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancel not propagated", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stream cancellation stalled")
	}

	waitCondition(t, func() bool { return docker.followActive.Load() == 0 })
	// A new subscription on the same connection can finish naturally.
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()

	var received atomic.Int64

	go func() {
		finished <- hub.Console(ctx, id, deployment.ServerID, func(event model.ConsoleEvent) error {
			if !utf8.ValidString(event.Content) {
				t.Error("console split a UTF-8 character")
			}

			received.Add(int64(len(event.Content)))
			time.Sleep(time.Millisecond)

			return nil
		})
	}()

	waitCondition(t, func() bool { return docker.followActive.Load() == 1 })

	select {
	case docker.console <- "burst":
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	select {
	case docker.console <- "end":
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}

		if received.Load() != int64(len("recent history\n")+len("\uFFFD")+128*len(strings.Repeat("🎮", 2049))) {
			t.Fatal("burst lost output", received.Load())
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	go func() {
		finished <- hub.Console(ctx, id, deployment.ServerID, func(model.ConsoleEvent) error { return nil })
	}()

	waitCondition(t, func() bool { return docker.followActive.Load() == 1 })
	hub.Disconnect(id)

	select {
	case err := <-finished:
		if !errors.Is(err, agents.ErrDisconnected) {
			t.Fatal("disconnect did not end subscription", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	waitCondition(t, func() bool { return docker.followActive.Load() == 0 })
	waitCondition(t, func() bool { return hub.Connected(id) })
}

func (d *wireDocker) Run(_ context.Context, _ io.Reader, args ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch args[0] {
	case "ps":
		if d.state == "" {
			return "", nil
		}

		return "container-id", nil
	case "inspect":
		if d.state == "" {
			return "no such object", errors.New("missing")
		}

		return d.state, nil
	case "create":
		d.state = "created"
	case "start":
		d.state = "running"
	case "stop", "kill":
		d.state = "exited"
	case "rm":
		d.state = ""
	case "logs":
		return "Ready", nil
	}

	return "", nil
}

func testWireRuntime(t *testing.T, hub *agents.Hub, id string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	deployment := deploymentFixture(id, "lifecycle")
	deployment.Ember.Readiness = &model.Readiness{Any: []model.ReadinessMatch{{Value: "^Ready$", Regex: true}}, StripANSI: true}

	deployment.Ember.Configs = []model.ConfigFile{{Path: "config/settings.json", Parser: "json", Create: true, Values: map[string]string{"network.port": "{{SERVER_PORT}}"}, Strings: []string{"network.port"}}}
	if _, err := hub.Deploy(ctx, id, deployment); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"install", "start", "stop", "update", "reinstall", "backup", "restore", "restart", "kill", "delete"} {
		if action == "restore" {
			file, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "read", Path: "world.bin"})
			if err != nil {
				t.Fatal(err)
			}

			_, err = hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "write", Path: "world.bin", Data: []byte("changed"), Revision: file.Revision})
			if err != nil {
				t.Fatal(err)
			}
		}

		content := ""
		if action == "restore" {
			content = "backup-operation.tar.gz"
		}

		status, err := hub.Action(ctx, id, model.RuntimeRequest{ServerID: deployment.ServerID, Action: action, OperationID: action + "-operation", Content: content})
		if err != nil {
			t.Fatal(err)
		}

		for status.Busy {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(5 * time.Millisecond):
			}

			status, err = hub.Action(ctx, id, model.RuntimeRequest{ServerID: deployment.ServerID, Action: "status"})
			if err != nil {
				t.Fatal(err)
			}
		}

		if status.Error != "" {
			t.Fatalf("remote %s: %s", action, status.Error)
		}

		if action == "install" {
			config, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "read", Path: "config/settings.json"})
			if err != nil || !strings.Contains(string(config.Data), `"25565"`) {
				t.Fatal("remote extended config missing", err, string(config.Data))
			}

			large := model.FileRequest{ServerID: deployment.ServerID, Operation: "upload.begin", Path: "large.bin", Revision: "absent", Size: 2 * model.FileLimit}

			upload, err := hub.Files(ctx, id, large)
			if err != nil {
				t.Fatal(err)
			}

			large.Operation, large.UploadID, large.Data = "upload.chunk", upload.UploadID, make([]byte, model.FileLimit)
			for _, offset := range []int64{0, model.FileLimit} {
				large.Offset = offset
				if _, err := hub.Files(ctx, id, large); err != nil {
					t.Fatal(err)
				}
			}

			large.Operation, large.Data = "upload.commit", nil
			if _, err := hub.Files(ctx, id, large); err != nil {
				t.Fatal(err)
			}

			info, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "stat", Path: "large.bin"})
			if err != nil || info.Size != 2*model.FileLimit {
				t.Fatal("remote large file stat", err, info.Size)
			}

			chunk, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "read.chunk", Path: "large.bin", Revision: info.Revision, Offset: model.FileLimit})
			if err != nil || len(chunk.Data) != model.FileLimit {
				t.Fatal("remote file chunk", err, len(chunk.Data))
			}

			if _, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "delete", Path: "large.bin"}); err != nil {
				t.Fatal(err)
			}

			request := model.FileRequest{ServerID: deployment.ServerID, Operation: "write", Path: "world.bin", Data: []byte{0, 255, 1}, Revision: "absent"}

			written, err := hub.Files(ctx, id, request)
			if err != nil {
				t.Fatal(err)
			}

			read, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "read", Path: "world.bin"})
			if err != nil || string(read.Data) != string(request.Data) || read.Revision != written.Revision {
				t.Fatalf("remote file transfer: %v", err)
			}

			request.Revision = "stale"
			_, err = hub.Files(ctx, id, request)

			var failure *model.FileError
			if !errors.As(err, &failure) || failure.Code != "file_conflict" {
				t.Fatalf("remote revision error mapping: %v", err)
			}
		}

		if action == "start" {
			if !status.Ready {
				t.Fatal("remote extended readiness did not match")
			}

			_, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "write", Path: "running.txt", Data: []byte("live"), Revision: "absent"})
			if err != nil {
				t.Fatalf("remote running file mutation: %v", err)
			}
		}

		if action == "backup" {
			listed, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "backup.list"})
			if err != nil || len(listed.Entries) != 1 {
				t.Fatalf("remote backup listing: %+v %v", listed, err)
			}

			data, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "backup.read", Path: "backup-operation.tar.gz"})
			if err != nil || len(data.Data) == 0 || data.Size != int64(len(data.Data)) {
				t.Fatal("remote backup download failed", err)
			}
		}

		if action == "restore" {
			file, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "read", Path: "world.bin"})
			if err != nil || string(file.Data) != string([]byte{0, 255, 1}) {
				t.Fatal("remote restore failed", err)
			}

			archive, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "backup.read", Path: "backup-operation.tar.gz"})
			if err != nil {
				t.Fatal(err)
			}

			objectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive.Data) }))
			defer objectServer.Close()

			if _, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "backup.delete", Path: "backup-operation.tar.gz"}); err != nil {
				t.Fatal(err)
			}

			hash := sha256.Sum256(archive.Data)

			remoteStatus, err := hub.Action(ctx, id, model.RuntimeRequest{ServerID: deployment.ServerID, Action: "restore", OperationID: "offhost-restore", Content: "backup-operation.tar.gz", BackupSource: &model.BackupSource{URL: objectServer.URL, Size: archive.Size, Digest: hex.EncodeToString(hash[:])}})
			if err != nil {
				t.Fatal(err)
			}

			for remoteStatus.Busy {
				time.Sleep(5 * time.Millisecond)

				remoteStatus, err = hub.Action(ctx, id, model.RuntimeRequest{ServerID: deployment.ServerID, Action: "status"})
				if err != nil {
					t.Fatal(err)
				}
			}

			if remoteStatus.Error != "" {
				t.Fatal("off-host restore over mTLS failed", remoteStatus.Error)
			}
		}

		if action == "delete" && status.State != "deleted" {
			t.Fatal("remote deletion was not acknowledged")
		}
	}
}
