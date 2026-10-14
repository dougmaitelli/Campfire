package spark

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"campfire/internal/model"
)

type wireDocker struct {
	mu    sync.Mutex
	state string
}

func (d *wireDocker) Run(_ context.Context, _ io.Reader, args ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch args[0] {
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

func testWireRuntime(t *testing.T, hub *Hub, id string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	deployment := deploymentFixture(id, "lifecycle")
	if _, err := hub.Deploy(ctx, id, deployment); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"install", "start", "stop", "restart", "kill", "delete"} {
		status, err := hub.Action(ctx, id, model.RuntimeRequest{ServerID: deployment.ServerID, Action: action, OperationID: action + "-operation"})
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
			_, err := hub.Files(ctx, id, model.FileRequest{ServerID: deployment.ServerID, Operation: "delete", Path: "world.bin"})

			var failure *model.FileError
			if !errors.As(err, &failure) || failure.Code != "file_busy" {
				t.Fatalf("remote running file guard: %v", err)
			}
		}

		if action == "delete" && status.State != "deleted" {
			t.Fatal("remote deletion was not acknowledged")
		}
	}
}
