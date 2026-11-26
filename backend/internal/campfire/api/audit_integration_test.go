package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func assertAuditOutcome(t *testing.T, f *accessFixture, action model.AuditAction, outcome model.AuditOutcome, target string) database.AuditEvent {
	t.Helper()

	var rows []database.AuditEvent
	if err := f.db.ORM().Where("action = ? AND outcome = ?", action, outcome).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}

	if len(rows) != 1 || !strings.HasPrefix(rows[0].Target, target) {
		t.Fatalf("expected one %s/%s on %s; got %+v", action, outcome, target, rows)
	}

	return rows[0]
}

func TestAuditLogicalFileActionsAndSensitiveMetadata(t *testing.T) {
	f := newAccessFixtureWithAudit(t, true)
	member, _ := f.user("audit-member", database.UserRole)
	_, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	endpoint := "/api/v1/servers/" + server.ID

	for _, operation := range []model.FileOperation{model.FileOperationUploadChunk, model.FileOperationStat, model.FileOperationList, model.FileOperationCompressStatus, model.FileOperationExtractStatus} {
		for range 3 {
			expectAccess(t, f.admin, "POST", endpoint+"/files", fmt.Sprintf(`{"operation":%q,"path":"world.dat"}`, operation), http.StatusOK)
		}
	}

	var noise int64
	if err := f.db.ORM().Model(&database.AuditEvent{}).Where("action LIKE ? OR action = ?", "server.files.%", "post.server").Count(&noise).Error; err != nil || noise != 0 {
		t.Fatal("routine file requests generated audit noise", noise, err)
	}

	// Raw API chunk reads do not have an enclosing audited download; keep
	// them visible so clients cannot bypass auditing by choosing this endpoint.
	expectAccess(t, f.admin, "POST", endpoint+"/files", `{"operation":"read.chunk","path":"world.dat"}`, http.StatusOK)
	assertAuditOutcome(t, f, "server.files.read.chunk", model.AuditSucceeded, "server:"+server.ID)
	expectAccess(t, f.admin, "POST", endpoint+"/files", `{"operation":"upload.commit","path":"world.dat"}`, http.StatusOK)
	assertAuditOutcome(t, f, "server.files.upload.commit", model.AuditSucceeded, "server:"+server.ID)
	f.runtime.files = func(model.FileRequest) (model.FileResult, error) {
		return model.FileResult{}, &model.FileError{Code: model.FileConflict, Message: "private failure payload"}
	}
	expectAccess(t, f.admin, "POST", endpoint+"/files", `{"operation":"upload.chunk","path":"world.dat"}`, http.StatusConflict)
	assertAuditOutcome(t, f, "server.files.upload.chunk", model.AuditFailed, "server:"+server.ID)
	expectAccess(t, f.admin, "POST", endpoint+"/files", `{"operation":"private-operation-input"}`, http.StatusBadRequest)
	assertAuditOutcome(t, f, "server.files.invalid", model.AuditFailed, "server:"+server.ID)
	expectAccess(t, f.admin, "POST", endpoint+"/console", `{"command":"private-console-command"}`, http.StatusOK)
	assertAuditOutcome(t, f, "server.command", model.AuditSucceeded, "server:"+server.ID)
	expectAccess(t, f.admin, "POST", endpoint+"/access/grants", fmt.Sprintf(`{"username":"audit-member","access_role_id":%d}`, database.OperatorRole), http.StatusNoContent)

	grant := assertAuditOutcome(t, f, "server.access.grant", model.AuditSucceeded, "server:"+server.ID)
	if !strings.Contains(grant.Target, "username:audit-member") || !strings.Contains(grant.Target, "access_role:") {
		t.Fatal("missing grant subject", grant.Target)
	}

	expectAccess(t, f.admin, "PUT", endpoint+"/owner", fmt.Sprintf(`{"user_id":%d}`, member.ID), http.StatusOK)

	owner := assertAuditOutcome(t, f, "server.owner.transfer", model.AuditSucceeded, "server:"+server.ID)
	if !strings.Contains(owner.Target, fmt.Sprintf("user:%d", member.ID)) {
		t.Fatal("missing owner", owner.Target)
	}

	var rows []database.AuditEvent
	if err := f.db.ORM().Find(&rows).Error; err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}

	for _, secret := range []string{"private-console-command", "private failure payload", "private-operation-input"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatal("audit retained content")
		}
	}
}
