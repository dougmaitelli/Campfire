package api_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestServerConflictResponsePreservesAllSpecificCauses(t *testing.T) {
	f := newAccessFixture(t)
	_, _, configuration := serverFixture(t, f.admin)
	expectAccess(t, f.admin, "POST", "/api/v1/servers", configJSON(t, configuration), http.StatusCreated)
	configuration.CPULimit = 2
	configuration.MemoryMB = 2048
	configuration.DiskMB = 20480

	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, configuration))
	if response.Code != http.StatusConflict {
		t.Fatal(response.Code, response.Body)
	}

	for _, message := range []string{"name already exists", "CPU capacity", "memory capacity", "disk capacity", "25565/tcp"} {
		if !strings.Contains(response.Body.String(), message) {
			t.Error("API discarded conflict details", message, response.Body)
		}
	}
}
