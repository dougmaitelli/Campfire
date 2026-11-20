package api_test

import (
	"net/http"
	"testing"

	"campfire/internal/campfire/api"
)

func TestAnonymousStatusReadsAreBoundedWithoutRequiringAuthentication(t *testing.T) {
	handler := api.New(nil, nil, nil).Handler()
	limited := false

	for range 200 {
		response := request(handler, "GET", "/api/v1/public/status-pages/missing", "")
		if response.Code == http.StatusTooManyRequests {
			if response.Header().Get("Retry-After") != "1" || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(response.Header())
			}

			limited = true

			break
		}

		if response.Code != http.StatusNotFound {
			t.Fatal(response.Code, response.Body)
		}
	}

	if !limited {
		t.Fatal("anonymous requests were not limited")
	}
	// Exhausting the anonymous budget does not put the private API behind that budget.
	private := request(handler, "GET", "/api/v1/servers", "")
	if private.Code != http.StatusServiceUnavailable {
		t.Fatal(private.Code, private.Body)
	}
}
