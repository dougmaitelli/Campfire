package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestServerActivityAccessLinksAndEvidenceRespectCurrentScopes(t *testing.T) {
	f := newAccessFixture(t)
	_, member := f.user("activity-reader", database.UserRole)
	_, outsider := f.user("activity-outsider", database.UserRole)
	_, token := createTestAPIToken(t, member, `["server.view"]`)
	viewer := bearerAPI(f.panel, token)
	node, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, f.admin, "POST", endpoint+"/members", `{"username":"activity-reader","role_id":5}`, http.StatusOK)

	alert := model.DetectedLogAlert{ID: strings.Repeat("a", 64), RuleKey: strings.Repeat("b", 64), ExecutionID: strings.Repeat("c", 64), Phase: model.LogRuntime, Title: "Action needed", Message: "Review settings", Severity: model.LogError, DetectedAt: time.Now().Unix(), Line: "secret-evidence"}
	if err := repository.NewLogAlertRepository(f.db.ORM()).Observe(t.Context(), node.ID, server.ID, model.LogObservation{Executions: map[model.LogPhase]string{model.LogRuntime: alert.ExecutionID}, Alerts: []model.DetectedLogAlert{alert}}, false); err != nil {
		t.Fatal(err)
	}

	response = request(viewer, "GET", endpoint+"/activity", "")

	var page repository.ServerActivityPage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].Kind != "alert" || page.Items[0].NotificationCount != 1 {
		t.Fatal("feed did not group the safe alert and panel notification", response.Code, response.Body)
	}

	key := page.Items[0].Key
	response = request(viewer, "GET", endpoint+"/activity/"+key, "")

	var details repository.ServerActivityDetails
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &details) != nil || strings.Contains(response.Body.String(), alert.Line) || strings.Contains(response.Body.String(), `"notifications"`) {
		t.Fatal("details leaked evidence or lost source links", response.Code, response.Body)
	}

	response = request(viewer, "GET", endpoint+"/activity/"+key+"/notifications", "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 1 || !page.Items[0].SourceAvailable || page.Items[0].SourceKey != key {
		t.Fatal("source query lost related notifications", response.Code, response.Body)
	}

	notificationKey := page.Items[0].Key
	expectAccess(t, viewer, "GET", endpoint+"/activity/"+notificationKey, "", http.StatusOK)
	expectAccess(t, viewer, "POST", endpoint+"/activity/"+notificationKey+"/retry", "{}", http.StatusForbidden)
	expectAccess(t, viewer, "GET", endpoint+"/activity/"+notificationKey+"/notifications", "", http.StatusNotFound)
	expectAccess(t, viewer, "GET", endpoint+"/activity/"+key+"/notifications?before=broken", "", http.StatusBadRequest)
	expectAccess(t, viewer, "GET", endpoint+"/activity/game_event:bad/notifications", "", http.StatusNotFound)

	response = request(f.admin, "GET", endpoint+"/activity/"+key, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), alert.Line) {
		t.Fatal("console reader lost evidence", response.Code, response.Body)
	}

	var read int64
	if err := f.db.ORM().Model(&database.NotificationRecipient{}).Where("read_at <> 0").Count(&read).Error; err != nil || read != 0 {
		t.Fatal("reading history marked personal notifications as read", read, err)
	}

	for _, path := range []string{"/activity", "/activity/filters", "/activity/" + key, "/activity/" + key + "/notifications"} {
		expectAccess(t, outsider, "GET", endpoint+path, "", http.StatusNotFound)
	}

	expectAccess(t, viewer, "GET", endpoint+"/activity/filters?kind=unknown", "", http.StatusBadRequest)

	for _, kind := range []string{"", "operations", "alert", "game"} {
		expectAccess(t, viewer, "GET", endpoint+"/activity/filters?kind="+kind, "", http.StatusOK)
	}

	for _, query := range []string{"?kind=unknown", "?kind=notification", "?before=broken", "?from=bad", "?from=-1", "?from=20&to=10", "?state=invalid", "?data.player=Alex&data.player=Sam", "?data.player=%00"} {
		expectAccess(t, viewer, "GET", endpoint+"/activity"+query, "", http.StatusBadRequest)
	}

	if err := f.db.ORM().Create(&database.ServerJob{ID: "private-transfer", Type: model.JobTransfer, SourceServerID: server.ID, TargetServerID: server.ID, SourceSparkID: node.ID, TargetSparkID: node.ID, Status: model.JobCompleted, Step: model.JobCleanup, CreatedAt: time.Now().Unix(), Digest: "private-digest"}).Error; err != nil {
		t.Fatal(err)
	}

	expectAccess(t, viewer, "GET", endpoint+"/activity/job:private-transfer", "", http.StatusNotFound)
	expectAccess(t, viewer, "GET", endpoint+"/activity/job:private-transfer/notifications", "", http.StatusNotFound)

	response = request(f.admin, "GET", endpoint+"/activity/job:private-transfer", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "private-digest") || !strings.Contains(response.Body.String(), `"can_retry":false`) {
		t.Fatal("job details lost authorization or exposed worker inputs", response.Code, response.Body)
	}
}
