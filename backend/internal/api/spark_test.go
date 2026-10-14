package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

	return api.New(service.NewSparkService(repository.NewSparkRepository(db.ORM()))).Handler(), closeDB
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

	response := request(handler, "POST", "/api/v1/sparks", `{"name":"  First Spark  ","kind":"remote"}`)
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

	if restored != spark {
		t.Fatalf("persisted Spark differs: %+v", restored)
	}

	response = request(handler, "GET", "/api/v1/sparks", "")

	var sparks []model.Spark
	if err := json.Unmarshal(response.Body.Bytes(), &sparks); err != nil {
		t.Fatal(err)
	}

	if response.Code != http.StatusOK || len(sparks) != 1 || sparks[0] != spark {
		t.Fatalf("list after reopen: %d %s", response.Code, response.Body)
	}
}

func TestSparkValidation(t *testing.T) {
	handler, _ := openAPI(t, filepath.Join(t.TempDir(), "panel.db"))
	for _, body := range []string{
		`{}`, `null`, `{"name":" ","kind":"remote"}`,
		`{"name":"node","kind":"invalid"}`,
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
		{`{"name":"local","kind":"local"}`, http.StatusCreated},
		{`{"name":"another local","kind":"local"}`, http.StatusConflict},
		{`{"name":"second remote","kind":"remote"}`, http.StatusCreated},
	} {
		response := request(handler, "POST", "/api/v1/sparks", test.body)
		if response.Code != test.status {
			t.Fatalf("%s: expected %d, got %d %s", test.body, test.status, response.Code, response.Body)
		}
	}
}

func TestConcurrentLocalSparkCreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	first, _ := openAPI(t, path)
	second, _ := openAPI(t, path)

	var wait sync.WaitGroup

	results := make(chan int, 2)

	for i, handler := range []http.Handler{first, second} {
		wait.Add(1)
		go func() {
			defer wait.Done()

			response := request(handler, "POST", "/api/v1/sparks", fmt.Sprintf(`{"name":"node-%d","kind":"local"}`, i))
			results <- response.Code
		}()
	}

	wait.Wait()
	close(results)

	counts := make(map[int]int)
	for status := range results {
		counts[status]++
	}

	if counts[http.StatusCreated] != 1 || counts[http.StatusConflict] != 1 {
		t.Fatalf("concurrent local creation: %v", counts)
	}
}
