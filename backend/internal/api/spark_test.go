package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"campfire/internal/api"
	"campfire/internal/database"
	"campfire/internal/database/repository"
	"campfire/internal/model"
	"campfire/internal/service"
)

func openAPI(t *testing.T, path string) (http.Handler, func()) {
	t.Helper()

	db, err := database.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	closeDB := func() {
		t.Helper()

		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(closeDB)

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)

	return authenticatedTestAPI(t, db, api.New(service.NewSparkService(repository.NewSparkRepository(db.ORM()), nil), embers, servers)), closeDB
}

func request(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	return response
}

func TestSparkPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")

	handler, closeDB := openAPI(t, path)
	if response := request(handler, "GET", "/api/v1/sparks", ""); response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("empty list: %d %s", response.Code, response.Body)
	}

	response := request(handler, "POST", "/api/v1/sparks", `{"name":"  First Spark  "}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body)
	}

	var spark model.Spark
	if err := json.Unmarshal(response.Body.Bytes(), &spark); err != nil {
		t.Fatal(err)
	}

	if spark.ID == "" || spark.Name != "First Spark" || spark.Kind != model.SparkRemote || spark.CreatedAt.IsZero() {
		t.Fatalf("invalid Spark: %+v", spark)
	}

	if response.Header().Get("Location") != "/api/v1/sparks/"+spark.ID {
		t.Fatal("missing resource location")
	}

	closeDB()

	handler, _ = openAPI(t, path)

	response = request(handler, "GET", "/api/v1/sparks/"+spark.ID, "")
	if response.Code != http.StatusOK {
		t.Fatalf("get after reopen: %d %s", response.Code, response.Body)
	}

	var restored model.Spark
	if err := json.Unmarshal(response.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(restored, spark) {
		t.Fatalf("persisted Spark differs: %+v", restored)
	}

	response = request(handler, "GET", "/api/v1/sparks", "")

	var sparks []model.Spark
	if err := json.Unmarshal(response.Body.Bytes(), &sparks); err != nil {
		t.Fatal(err)
	}

	if response.Code != http.StatusOK || len(sparks) != 1 || !reflect.DeepEqual(sparks[0], spark) {
		t.Fatalf("list after reopen: %d %s", response.Code, response.Body)
	}
}

func TestSparkValidation(t *testing.T) {
	handler, _ := openAPI(t, filepath.Join(t.TempDir(), "panel.db"))
	for _, body := range []string{
		`{}`, `null`, `{"name":" ","kind":"remote"}`,
		`{"name":"node","kind":"invalid"}`,
		`{"name":"node","kind":"local"}`,
		`{"name":"node\nname","kind":"remote"}`,
		`{"name":"node","kind":"remote","unknown":true}`,
		`{"name":"node","kind":"remote"} {}`,
		`{"name":`,
		fmt.Sprintf(`{"name":%q,"kind":"remote"}`, strings.Repeat("a", 101)),
	} {
		t.Run(body, func(t *testing.T) {
			response := request(handler, "POST", "/api/v1/sparks", body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", response.Code, response.Body)
			}
		})
	}

	response := request(handler, "POST", "/api/v1/sparks", strings.Repeat("x", 17000))
	if response.Code != http.StatusBadRequest && response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized malformed request: %d", response.Code)
	}

	response = request(handler, "POST", "/api/v1/sparks", fmt.Sprintf(`{"name":%q,"kind":"remote"}`, strings.Repeat("a", 17000)))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized JSON: %d", response.Code)
	}

	req := httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/sparks", bytes.NewBufferString(`{"name":"node","kind":"remote"}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("missing content type: %d", response.Code)
	}

	response = request(handler, "GET", "/api/v1/sparks/missing", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing Spark: %d", response.Code)
	}

	response = request(handler, "GET", "/api/v1/sparks", "")
	if strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("invalid requests inserted records: %s", response.Body)
	}
}

func TestSparkConflicts(t *testing.T) {
	handler, _ := openAPI(t, filepath.Join(t.TempDir(), "panel.db"))
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"name":"remote","kind":"remote"}`, http.StatusCreated},
		{`{"name":"REMOTE","kind":"remote"}`, http.StatusConflict},
		{`{"name":"second remote","kind":"remote"}`, http.StatusCreated},
	} {
		response := request(handler, "POST", "/api/v1/sparks", test.body)
		if response.Code != test.status {
			t.Fatalf("%s: expected %d, got %d %s", test.body, test.status, response.Code, response.Body)
		}
	}
}

func TestConcurrentAgentSparkCreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	first, _ := openAPI(t, path)
	second, _ := openAPI(t, path)

	var wait sync.WaitGroup

	results := make(chan int, 2)

	for i, handler := range []http.Handler{first, second} {
		wait.Add(1)
		go func() {
			defer wait.Done()

			response := request(handler, "POST", "/api/v1/sparks", fmt.Sprintf(`{"name":"node-%d"}`, i))
			results <- response.Code
		}()
	}

	wait.Wait()
	close(results)

	counts := make(map[int]int)
	for status := range results {
		counts[status]++
	}

	if counts[http.StatusCreated] != 2 {
		t.Fatalf("concurrent agent creation: %v", counts)
	}
}

func TestSparkDescriptionAndDisabledUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	handler, closeDB := openAPI(t, path)

	response := request(handler, "POST", "/api/v1/sparks", `{"name":"node","kind":"remote","description":"  Rack one\nShelf two  ","disabled":true}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body)
	}

	var original model.Spark
	if err := json.Unmarshal(response.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}

	if original.Description != "Rack one\nShelf two" || !original.Disabled || original.Connected {
		t.Fatalf("unexpected initial state: %+v", original)
	}

	endpoint := "/api/v1/sparks/" + original.ID
	for _, body := range []string{`{"description":"Updated"}`, `{"disabled":false}`} {
		response = request(handler, "PATCH", endpoint, body)
		if response.Code != http.StatusOK {
			t.Fatalf("update %s: %d %s", body, response.Code, response.Body)
		}
	}

	closeDB()

	handler, _ = openAPI(t, path)
	response = request(handler, "GET", endpoint, "")

	var updated model.Spark
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}

	if updated.Description != "Updated" || updated.Disabled || updated.Connected || updated.Name != original.Name || updated.Kind != original.Kind || updated.CreatedAt != original.CreatedAt {
		t.Fatalf("update did not persist correctly: %+v", updated)
	}

	response = request(handler, "PATCH", endpoint, `{"description":""}`)
	if response.Code != http.StatusOK {
		t.Fatalf("clear description: %d %s", response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}

	if updated.Description != "" || updated.Disabled {
		t.Fatalf("failed to clear description: %+v", updated)
	}

	for _, body := range []string{
		`{}`, `null`, `{"connected":true}`, `{"disabled":"yes"}`,
		`{"description":"bad\u0000text"}`,
		fmt.Sprintf(`{"description":%q}`, strings.Repeat("a", 1001)),
	} {
		response = request(handler, "PATCH", endpoint, body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid update %s: %d %s", body, response.Code, response.Body)
		}
	}

	response = request(handler, "PATCH", "/api/v1/sparks/missing", `{"disabled":true}`)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing Spark: %d", response.Code)
	}
}

func TestSparkCapacityAndNetworking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	handler, closeDB := openAPI(t, path)

	response := request(handler, "POST", "/api/v1/sparks", `{
		"name":"Game host","kind":"remote","game_address":" Games.Example.COM ",
		"cpu_limit":4.5,"memory_mb":8192,"disk_mb":102400,
		"port_allocations":[
			{"bind_ip":"0.0.0.0","start":25565,"end":25570,"protocol":"tcp"},
			{"bind_ip":"0.0.0.0","start":25565,"end":25570,"protocol":"udp"}
		]
	}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body)
	}

	var original model.Spark
	if err := json.Unmarshal(response.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}

	if original.GameAddress != "games.example.com" || original.CPULimit != 4.5 || original.MemoryMB != 8192 || original.DiskMB != 102400 || len(original.PortAllocations) != 2 {
		t.Fatalf("incorrect settings: %+v", original)
	}

	endpoint := "/api/v1/sparks/" + original.ID

	closeDB()

	handler, _ = openAPI(t, path)
	response = request(handler, "GET", endpoint, "")

	var restored model.Spark
	if err := json.Unmarshal(response.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(original, restored) {
		t.Fatalf("settings changed after reopen: %+v", restored)
	}

	for _, body := range []string{
		`{"game_address":"https://games.example.com"}`,
		`{"game_address":"games.example.com:25565"}`,
		`{"cpu_limit":-1}`, `{"memory_mb":-1}`, `{"disk_mb":1.5}`,
		`{"port_allocations":[{"bind_ip":"domain.example","start":1,"end":10,"protocol":"tcp"}]}`,
		`{"port_allocations":[{"bind_ip":"0.0.0.0","start":0,"end":10,"protocol":"tcp"}]}`,
		`{"port_allocations":[{"bind_ip":"0.0.0.0","start":10,"end":1,"protocol":"tcp"}]}`,
		`{"port_allocations":[{"bind_ip":"0.0.0.0","start":1,"end":65536,"protocol":"udp"}]}`,
		`{"port_allocations":[{"bind_ip":"0.0.0.0","start":1,"end":10,"protocol":"http"}]}`,
		`{"port_allocations":[{"bind_ip":"0.0.0.0","start":1,"end":10,"protocol":"tcp"},{"bind_ip":"192.0.2.1","start":10,"end":20,"protocol":"tcp"}]}`,
	} {
		response = request(handler, "PATCH", endpoint, body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid settings %s: %d %s", body, response.Code, response.Body)
		}
	}

	response = request(handler, "PATCH", endpoint, `{"memory_mb":0,"port_allocations":[],"game_address":""}`)
	if response.Code != http.StatusOK {
		t.Fatalf("clear settings: %d %s", response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), &restored); err != nil {
		t.Fatal(err)
	}

	if restored.GameAddress != "" || restored.MemoryMB != 0 || len(restored.PortAllocations) != 0 || restored.CPULimit != 4.5 || restored.DiskMB != 102400 {
		t.Fatalf("partial update incorrect: %+v", restored)
	}

	response = request(handler, "PATCH", endpoint, `{"game_address":"2001:db8::1","port_allocations":[{"bind_ip":"::","start":27015,"end":27015,"protocol":"udp"}]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("IPv6 settings: %d %s", response.Code, response.Body)
	}
}
