package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestCrashActivityRequiresCurrentServerAndConsolePermissions(t *testing.T) {
	f := newAccessFixture(t)
	user, member := f.user("crash-reader", database.UserRole)
	_, outsider := f.user("crash-outsider", database.UserRole)
	_, token := createTestAPIToken(t, member, `["server.view"]`)
	viewer := bearerAPI(f.panel, token)
	node, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, f.admin, "POST", endpoint+"/members", `{"username":"crash-reader","role_id":5}`, http.StatusOK)

	record := model.CrashRecord{ID: strings.Repeat("a", 64), ContainerID: strings.Repeat("b", 64), ExitedAt: time.Now().Unix(), ExitCode: 137, Reason: model.CrashReasonOutOfMemory, Console: "private-crash-console"}
	if err := repository.NewCrashRepository(f.db.ORM()).Observe(t.Context(), node.ID, server.ID, []model.CrashRecord{record}); err != nil {
		t.Fatal(err)
	}

	path := endpoint + "/activity/crash:" + record.ID

	response = request(viewer, "GET", path, "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), record.Console) || !strings.Contains(response.Body.String(), `"exit_code":137`) {
		t.Fatal(response.Code, response.Body)
	}

	response = request(f.admin, "GET", path, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), record.Console) {
		t.Fatal(response.Code, response.Body)
	}

	response = request(viewer, "GET", endpoint+"/activity?kind=alert", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), record.Console) || !strings.Contains(response.Body.String(), "crash:"+record.ID) {
		t.Fatal(response.Code, response.Body)
	}

	response = request(viewer, "GET", endpoint+"/activity?search=private-crash-console", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "crash:"+record.ID) {
		t.Fatal("search leaked console evidence", response.Code, response.Body)
	}

	expectAccess(t, outsider, "GET", path, "", http.StatusNotFound)
	expectAccess(t, outsider, "GET", endpoint+"/activity?kind=alert", "", http.StatusNotFound)
	expectAccess(t, viewer, "GET", endpoint+"/activity/crash:"+strings.Repeat("c", 64), "", http.StatusNotFound)
	// Revocation is checked at read time, including already known crash IDs.
	expectAccess(t, f.admin, "DELETE", endpoint+"/members/"+strconv.FormatInt(user.ID, 10), "", http.StatusNoContent)
	expectAccess(t, viewer, "GET", path, "", http.StatusNotFound)
}
