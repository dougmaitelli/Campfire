package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestBackupPolicyDeletionAndPermissionBoundaries(t *testing.T) {
	f := newAccessFixture(t)
	user, owner := f.user("owner", database.UserRole)
	_, operator := f.user("operator", database.UserRole)
	_, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server

	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, f.admin, "PUT", endpoint+"/owner", fmt.Sprintf(`{"user_id":%d}`, user.ID), http.StatusOK)
	expectAccess(t, owner, "POST", endpoint+"/members", `{"username":"operator","role_id":5}`, http.StatusOK)

	deleted := false
	f.runtime.files = func(q model.FileRequest) (model.FileResult, error) {
		result := model.FileResult{ServerID: q.ServerID, Path: q.Path}
		if q.Operation == "backup.list" && !deleted {
			result.Entries = []model.FileEntry{{Name: "snapshot.tar.gz", Kind: "file", Size: 4, ModifiedAt: time.Now()}}
		}

		if q.Operation == "backup.delete" {
			deleted = true
		}

		return result, nil
	}
	policy := `{"storage":"local","local_keep_last":3,"local_max_age_days":0,"remote_keep_last":1,"remote_max_age_days":0}`
	expectAccess(t, operator, "PUT", endpoint+"/backup-policy", policy, http.StatusForbidden)
	expectAccess(t, operator, "DELETE", endpoint+"/backups/snapshot.tar.gz", "", http.StatusForbidden)
	expectAccess(t, operator, "POST", endpoint+"/files", `{"operation":"backup.delete","path":"snapshot.tar.gz"}`, http.StatusBadRequest)
	expectAccess(t, owner, "PUT", endpoint+"/backup-policy", strings.Replace(policy, `"local"`, `"s3"`, 1), http.StatusBadRequest)
	expectAccess(t, owner, "PUT", endpoint+"/backup-policy", policy, http.StatusOK)

	response = request(operator, "GET", endpoint+"/backups", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"local":true`) || !strings.Contains(response.Body.String(), `"local_keep_last":3`) {
		t.Fatal("catalog or retention missing", response.Code, response.Body)
	}

	expectAccess(t, owner, "DELETE", endpoint+"/backups/snapshot.tar.gz?storage=invalid", "", http.StatusBadRequest)
	expectAccess(t, owner, "DELETE", endpoint+"/backups/snapshot.tar.gz?storage=local", "", http.StatusAccepted)

	if !deleted {
		t.Fatal("local backup was not deleted")
	}

	response = request(owner, "GET", endpoint+"/backups", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "snapshot.tar.gz") {
		t.Fatal("deleted backup remained in catalog", response.Code, response.Body)
	}
}
