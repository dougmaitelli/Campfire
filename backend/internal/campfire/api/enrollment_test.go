package api_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"campfire/internal/campfire/agents"
	"campfire/internal/campfire/api"
	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
)

func TestSparkInstallationEnrollmentResponses(t *testing.T) {
	for _, configured := range []bool{false, true} {
		name := "listener disabled"
		if configured {
			name = "listener enabled"
		}

		t.Run(name, func(t *testing.T) {
			db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "panel.db"))
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = db.Close() })

			sparks := service.NewSparkService(repository.NewSparkRepository(db.ORM()), nil)
			embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
			servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)

			panel := api.New(sparks, embers, servers)
			if configured {
				panel.WithAgentHub(agents.NewHub(repository.NewEnrollmentRepository(db.ORM()), nil).WithEnrollmentEndpoint("https://agents.test:8443", func() []byte { return []byte("public test CA") }))
			}

			handler := authenticatedTestAPI(t, db, panel)

			created := request(handler, "POST", "/api/v1/sparks", `{"name":"Install test"}`)
			if created.Code != http.StatusCreated {
				t.Fatalf("create: %d %s", created.Code, created.Body)
			}

			response := request(handler, "POST", created.Header().Get("Location")+"/enrollment", "{}")
			if response.Header().Get("Content-Type") != "application/json" {
				t.Fatal("enrollment returned non-JSON response")
			}

			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}

			if configured {
				if response.Code != http.StatusCreated || body["token"] == nil || body["expires_at"] == nil {
					t.Fatal("missing installation enrollment response", response.Code)
				}

				if body["agent_url"] != "https://agents.test:8443" || body["server_ca"] != "public test CA" {
					t.Fatal("installation response omitted public TLS metadata")
				}
			} else if response.Code != http.StatusServiceUnavailable || body["error"] != "the panel agent HTTPS listener is not configured" {
				t.Fatal("missing listener configuration error", response.Code)
			}
		})
	}
}
