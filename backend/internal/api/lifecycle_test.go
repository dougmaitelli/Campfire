package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"campfire/internal/api"
	"campfire/internal/database"
	"campfire/internal/database/repository"
	"campfire/internal/model"
	"campfire/internal/runtime"
	"campfire/internal/service"
	"campfire/internal/spark"
)

type lifecycleDocker struct {
	mu    sync.Mutex
	state string
	fail  bool
}

func (d *lifecycleDocker) Run(_ context.Context, _ io.Reader, args ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.fail {
		return "", errors.New("Docker unavailable")
	}

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

func TestLifecycleAPIAndDeletionReservations(t *testing.T) {
	directory := t.TempDir()

	db, err := database.Open(context.Background(), filepath.Join(directory, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), nil)
	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	docker := &lifecycleDocker{}
	manager := runtime.New(context.Background(), filepath.Join(directory, "runtime"))
	manager.Docker = docker

	manager.Ownership = func(string, string) error { return nil }

	manager.FileOwner = func(*os.File, string) error { return nil }
	defer manager.Close()

	deployment := &service.DeploymentService{Servers: servers, Sparks: sparks, Embers: embers, Local: &spark.ConfigurationStore{Directory: filepath.Join(directory, "servers"), Runtime: manager}}
	handler := api.New(sparks, embers, servers).WithDeployment(deployment).Handler()
	_, _, input := serverFixture(t, handler)

	response := request(handler, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Body)
	}

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID

	var backupName string

	for _, action := range []string{"install", "start", "stop", "backup", "restore", "restart", "kill"} {
		response = request(handler, "POST", endpoint+"/actions", `{"action":"`+action+`","backup":"`+backupName+`"}`)
		if response.Code != http.StatusAccepted {
			t.Fatalf("%s: %d %s", action, response.Code, response.Body)
		}

		deadline := time.Now().Add(2 * time.Second)

		for {
			response = request(handler, "GET", endpoint+"/status", "")
			if response.Code != http.StatusOK {
				t.Fatal(response.Body)
			}

			var status model.RuntimeStatus
			if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}

			if !status.Busy {
				if status.Error != "" {
					t.Fatal(status.Error)
				}

				break
			}

			if time.Now().After(deadline) {
				t.Fatal("operation timed out")
			}

			time.Sleep(5 * time.Millisecond)
		}

		if action == "start" {
			response = request(handler, "POST", endpoint+"/files", `{"operation":"delete","path":"test.txt"}`)
			if response.Code != http.StatusConflict {
				t.Fatalf("running file mutation: %d %s", response.Code, response.Body)
			}

			response = request(handler, "PUT", endpoint, configJSON(t, input))
			if response.Code != http.StatusConflict {
				t.Fatalf("running server edit changed reservations: %d", response.Code)
			}

			response = request(handler, "GET", endpoint+"/console", "")
			if response.Code != http.StatusOK {
				t.Fatalf("console: %d %s", response.Code, response.Body)
			}
		}

		if action == "stop" {
			response = request(handler, "PUT", endpoint, configJSON(t, input))
			if response.Code != http.StatusOK {
				t.Fatalf("stopped server edit failed: %d %s", response.Code, response.Body)
			}
		}

		if action == "backup" {
			response = request(handler, "GET", endpoint+"/backups", "")

			var backups model.FileResult
			if err := json.Unmarshal(response.Body.Bytes(), &backups); err != nil || response.Code != http.StatusOK || len(backups.Entries) != 1 {
				t.Fatalf("backup listing: %d %s %v", response.Code, response.Body, err)
			}

			backupName = backups.Entries[0].Name

			response = request(handler, "GET", endpoint+"/backups/"+backupName, "")
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/gzip" || response.Body.Len() == 0 {
				t.Fatalf("backup download: %d %s", response.Code, response.Body)
			}

			large := bytes.Repeat([]byte("archive"), model.FileLimit/7+100)
			if err := os.WriteFile(filepath.Join(directory, "runtime", server.ID, "backups", "large.tar.gz"), large, 0600); err != nil {
				t.Fatal(err)
			}

			response = request(handler, "GET", endpoint+"/backups/large.tar.gz", "")
			if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), large) {
				t.Fatalf("multi-chunk backup download: %d, %d bytes", response.Code, response.Body.Len())
			}

			response = request(handler, "GET", endpoint+"/backups/missing.tar.gz", "")
			if response.Code != http.StatusNotFound {
				t.Fatalf("missing backup download: %d", response.Code)
			}

			if err := os.WriteFile(filepath.Join(directory, "runtime", server.ID, "data", "test.txt"), []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
		}

		if action == "restore" {
			data, err := os.ReadFile(filepath.Join(directory, "runtime", server.ID, "data", "test.txt"))
			if err != nil || string(data) != "hello" {
				t.Fatal("API restore failed", err)
			}
		}

		if action == "install" {
			response = request(handler, "POST", endpoint+"/files", `{"server_id":"forged","operation":"write","path":"test.txt","data":"aGVsbG8=","revision":"absent"}`)
			if response.Code != http.StatusOK {
				t.Fatalf("upload: %d %s", response.Code, response.Body)
			}

			response = request(handler, "GET", endpoint+"/files", "")
			if response.Code != http.StatusOK {
				t.Fatalf("list: %d %s", response.Code, response.Body)
			}

			response = request(handler, "POST", endpoint+"/files", `{"operation":"read","path":"test.txt"}`)
			if response.Code != http.StatusOK {
				t.Fatalf("read: %d %s", response.Code, response.Body)
			}

			var file model.FileResult
			if err := json.Unmarshal(response.Body.Bytes(), &file); err != nil {
				t.Fatal(err)
			}

			if file.ServerID != server.ID || string(file.Data) != "hello" {
				t.Fatal("file request changed assigned server")
			}

			response = request(handler, "POST", endpoint+"/files", `{"operation":"write","path":"test.txt","data":"aGVsbG8=","revision":"stale"}`)
			if response.Code != http.StatusConflict {
				t.Fatalf("stale edit: %d", response.Code)
			}

			response = request(handler, "POST", endpoint+"/files", `{"operation":"read","path":"../state.json"}`)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("path traversal: %d", response.Code)
			}
		}
	}

	docker.mu.Lock()
	docker.fail = true
	docker.mu.Unlock()

	response = request(handler, "DELETE", endpoint, "")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("offline delete: %d", response.Code)
	}

	if _, err := servers.Get(context.Background(), server.ID); err != nil {
		t.Fatal("failed cleanup removed database reservation")
	}

	input.Name = "Conflicting reservation"

	response = request(handler, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusConflict {
		t.Fatalf("failed cleanup released ports: %d", response.Code)
	}

	docker.mu.Lock()
	docker.fail = false
	docker.mu.Unlock()
	// The failed cleanup is an asynchronous operation; inspect its completion before retrying.
	deadline := time.Now().Add(time.Second)

	for {
		response = request(handler, "GET", endpoint+"/status", "")

		var status model.RuntimeStatus
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}

		if !status.Busy {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("failed cleanup did not finish")
		}

		time.Sleep(5 * time.Millisecond)
	}

	response = request(handler, "DELETE", endpoint, "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", response.Code, response.Body)
	}

	response = request(handler, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatalf("resources not released after cleanup: %d %s", response.Code, response.Body)
	}
}
