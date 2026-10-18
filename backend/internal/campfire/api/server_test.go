package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"campfire/internal/shared/model"
)

func serverFixture(t *testing.T, handler http.Handler) (model.Spark, model.Ember, model.ServerConfiguration) {
	t.Helper()
	return serverFixtureKind(t, handler, "remote")
}

func serverFixtureKind(t *testing.T, handler http.Handler, _ string) (model.Spark, model.Ember, model.ServerConfiguration) {
	t.Helper()

	response := request(handler, "POST", "/api/v1/sparks", `{"name":"Game node","cpu_limit":2,"memory_mb":2048,"disk_mb":20480,"port_allocations":[{"bind_ip":"0.0.0.0","start":25565,"end":25570,"protocol":"tcp"}]}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("Spark: %d %s", response.Code, response.Body)
	}

	var spark model.Spark
	if err := json.Unmarshal(response.Body.Bytes(), &spark); err != nil {
		t.Fatal(err)
	}

	response = request(handler, "POST", "/api/v1/embers", `{"schema":1,"name":"Game","description":"","images":{"Alpine":"alpine:3.22"},"startup":"sleep {{SECONDS}}","work_dir":"/home/container","user":"1000:1000","variables":[{"name":"SECONDS","default":"3600","required":true,"editable":true,"pattern":"^[0-9]+$"}],"required_ports":[{"container":25565,"protocol":"tcp"}]}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("Ember: %d %s", response.Code, response.Body)
	}

	var ember model.Ember
	if err := json.Unmarshal(response.Body.Bytes(), &ember); err != nil {
		t.Fatal(err)
	}

	input := model.ServerConfiguration{Name: "First server", SparkID: spark.ID, EmberID: ember.ID, Image: "alpine:3.22", CPULimit: 1, MemoryMB: 1024, DiskMB: 10240, Ports: []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25565, ContainerPort: 25565, Protocol: "tcp"}}}

	return spark, ember, input
}

func configJSON(t *testing.T, input model.ServerConfiguration) string {
	t.Helper()

	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	return string(body)
}

func TestServerConfigurationLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	handler, closeDB := openAPI(t, path)
	spark, ember, input := serverFixture(t, handler)

	response := request(handler, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body)
	}

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	if server.Status != "not_installed" || server.SparkName != spark.Name || server.EmberName != ember.Name || server.Variables["SECONDS"] != "3600" {
		t.Fatalf("incorrect server: %+v", server)
	}

	closeDB()

	handler, _ = openAPI(t, path)
	endpoint := "/api/v1/servers/" + server.ID
	response = request(handler, "GET", endpoint, "")

	var restored model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(restored, server) {
		t.Fatalf("persistence mismatch: %+v", restored)
	}

	response = request(handler, "PATCH", "/api/v1/sparks/"+spark.ID, `{"port_allocations":[]}`)
	if response.Code != http.StatusConflict {
		t.Fatalf("removed reserved ports: %d %s", response.Code, response.Body)
	}

	response = request(handler, "PATCH", "/api/v1/sparks/"+spark.ID, `{"memory_mb":512}`)
	if response.Code != http.StatusConflict {
		t.Fatalf("shrunk reserved capacity: %d %s", response.Code, response.Body)
	}

	input.Name = "Renamed server"
	input.Description = "Updated configuration"
	input.Variables = map[string]string{"SECONDS": "7200"}
	input.Ports[0].HostPort = 25566

	response = request(handler, "PUT", endpoint, configJSON(t, input))
	if response.Code != http.StatusOK {
		t.Fatalf("update: %d %s", response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}

	if restored.Name != input.Name || restored.Variables["SECONDS"] != "7200" || restored.Ports[0].HostPort != 25566 || !restored.CreatedAt.Equal(server.CreatedAt) {
		t.Fatalf("update failed: %+v", restored)
	}

	response = request(handler, "DELETE", endpoint, "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", response.Code, response.Body)
	}

	response = request(handler, "GET", endpoint, "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("deleted server still exists: %d", response.Code)
	}

	response = request(handler, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatalf("reservations were not released: %d %s", response.Code, response.Body)
	}
}

func TestServerValidationAndReservations(t *testing.T) {
	handler, _ := openAPI(t, filepath.Join(t.TempDir(), "panel.db"))

	spark, _, input := serverFixture(t, handler)
	for _, modify := range []func(*model.ServerConfiguration){
		func(c *model.ServerConfiguration) { c.Image = "unapproved:latest" },
		func(c *model.ServerConfiguration) { c.Variables = map[string]string{"UNKNOWN": "value"} },
		func(c *model.ServerConfiguration) { c.Variables = map[string]string{"SECONDS": "invalid"} },
		func(c *model.ServerConfiguration) { c.CPULimit = -1 },
		func(c *model.ServerConfiguration) { c.MemoryMB = 0 },
		func(c *model.ServerConfiguration) { c.Ports = []model.ServerPort{} },
		func(c *model.ServerConfiguration) {
			c.Ports = []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 30000, ContainerPort: 25565, Protocol: "tcp"}}
		},
		func(c *model.ServerConfiguration) {
			c.Ports = []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25565, ContainerPort: 25566, Protocol: "tcp"}}
		},
	} {
		candidate := input
		modify(&candidate)

		response := request(handler, "POST", "/api/v1/servers", configJSON(t, candidate))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid config accepted: %d %s", response.Code, response.Body)
		}
	}

	response := request(handler, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatalf("first reservation: %d %s", response.Code, response.Body)
	}

	input.Name = "Second server"

	response = request(handler, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate port allowed: %d", response.Code)
	}

	input.Ports[0].HostPort = 25566
	input.MemoryMB = 1536

	response = request(handler, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusConflict {
		t.Fatalf("overbooked memory: %d", response.Code)
	}

	response = request(handler, "PATCH", "/api/v1/sparks/"+spark.ID, `{"disabled":true}`)
	if response.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", response.Code, response.Body)
	}

	input.MemoryMB = 1024

	response = request(handler, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("placement on disabled Spark allowed: %d", response.Code)
	}
}

func TestConcurrentServerReservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	first, _ := openAPI(t, path)
	_, _, input := serverFixture(t, first)
	second, _ := openAPI(t, path)

	var wait sync.WaitGroup

	results := make(chan int, 2)

	for i, handler := range []http.Handler{first, second} {
		candidate := input
		candidate.Name = fmt.Sprintf("Server %d", i)
		body := configJSON(t, candidate)

		wait.Add(1)
		go func() { defer wait.Done(); results <- request(handler, "POST", "/api/v1/servers", body).Code }()
	}

	wait.Wait()
	close(results)

	counts := map[int]int{}
	for status := range results {
		counts[status]++
	}

	if counts[http.StatusCreated] != 1 || counts[http.StatusConflict] != 1 {
		t.Fatalf("non-atomic reservations: %v", counts)
	}
}

func TestInvalidEmberImport(t *testing.T) {
	handler, _ := openAPI(t, filepath.Join(t.TempDir(), "panel.db"))
	for _, body := range []string{
		`{"schema":2,"name":"bad"}`,
		`{"schema":1,"name":"root","images":{"image":"alpine:3.22"},"startup":"sleep 1","work_dir":"/home/container","user":"0","variables":[]}`,
		`{"schema":1,"name":"unknown","images":{"image":"alpine:3.22"},"startup":"echo {{UNKNOWN}}","work_dir":"/home/container","user":"1000","variables":[]}`,
	} {
		response := request(handler, "POST", "/api/v1/embers", body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid Ember accepted: %d %s", response.Code, response.Body)
		}
	}
}
