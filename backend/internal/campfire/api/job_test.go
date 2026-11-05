package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestSharedJobHistoryFiltersBeforePaginationAndHidesPrivateData(t *testing.T) {
	f := newAccessFixture(t)
	owner, regular := f.user("job-owner", database.CreatorRole)
	_, outsider := f.user("job-outsider", database.CreatorRole)
	spark, _, input := serverFixture(t, f.admin)

	body, err := json.Marshal(struct {
		model.ServerConfiguration
		OwnerUserID int64 `json:"owner_user_id"`
	}{input, owner.ID})
	if err != nil {
		t.Fatal(err)
	}

	created := request(f.admin, "POST", "/api/v1/servers", string(body))

	var server model.Server
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &server) != nil {
		t.Fatal(created.Code, created.Body)
	}

	jobs := make([]database.ServerJob, 0, 61)
	for index := 0; index < 60; index++ {
		jobs = append(jobs, database.ServerJob{ID: fmt.Sprintf("transfer-%03d", index), Type: model.JobTransfer, SourceServerID: server.ID, TargetServerID: server.ID, SourceSparkID: spark.ID, TargetSparkID: spark.ID, Data: `{"source_ports":[],"target_ports":[]}`, Status: model.JobCompleted, Step: model.JobCleanup, Attempt: 1, CreatedAt: int64(100 + index), Digest: "private-archive-digest"})
	}

	jobs = append(jobs, database.ServerJob{ID: "historical-copy", Type: model.JobCopy, SourceServerID: server.ID, TargetServerID: "deleted-copy", SourceSparkID: spark.ID, TargetSparkID: spark.ID, Data: `{}`, Status: model.JobCancelled, Step: model.JobPreparing, Attempt: 1, CreatedAt: 1})
	if err := f.db.ORM().Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/servers/" + server.ID + "/jobs"
	response := request(regular, "GET", path, "")

	var page struct {
		Items []database.ServerJob `json:"items"`
		Next  string               `json:"next"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].ID != "historical-copy" || page.Next != "" {
		t.Fatal("permissions were applied after pagination", response.Code, response.Body)
	}

	expectAccess(t, outsider, "GET", path, "", http.StatusNotFound)
	expectAccess(t, regular, "POST", "/api/v1/jobs/transfer-000/retry", "{}", http.StatusNotFound)
	expectAccess(t, f.admin, "GET", path+"?before=invalid", "", http.StatusBadRequest)

	response = request(f.admin, "GET", path, "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 50 || page.Next == "" {
		t.Fatal("first history page is invalid", response.Code, response.Body)
	}

	for _, private := range []string{"private-archive-digest", "source_ports", "target_ports", "attempt", "data"} {
		if strings.Contains(response.Body.String(), `"`+private+`"`) {
			t.Fatal("job history exposed private data", private)
		}
	}

	seen := make(map[string]bool, 61)
	for _, job := range page.Items {
		seen[job.ID] = true
	}

	response = request(f.admin, "GET", path+"?before="+page.Next, "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 11 || page.Next != "" {
		t.Fatal("second history page is invalid", response.Code, response.Body)
	}

	for _, job := range page.Items {
		if seen[job.ID] {
			t.Fatal("history cursor repeated a job", job.ID)
		}
	}
}

func TestPlatformJobHistoryRequiresAdminAndFiltersAcrossServers(t *testing.T) {
	f := newAccessFixture(t)
	_, regular := f.user("jobs-regular", database.CreatorRole)
	expectAccess(t, regular, "GET", "/api/v1/jobs", "", http.StatusForbidden)
	_, secret := createTestAPIToken(t, f.admin, `["audit.view"]`)
	expectAccess(t, bearerAPI(f.panel, secret), "GET", "/api/v1/jobs", "", http.StatusForbidden)

	jobs := make([]database.ServerJob, 0, 71)

	for index := 0; index < 70; index++ {
		kind := model.JobCopy
		if index%2 == 0 {
			kind = model.JobTransfer
		}

		jobs = append(jobs, database.ServerJob{ID: fmt.Sprintf("history-%03d", index), Type: kind, SourceServerID: fmt.Sprintf("deleted-source-%d", index), TargetServerID: fmt.Sprintf("deleted-target-%d", index), SourceSparkID: "deleted-source-spark", TargetSparkID: "deleted-target-spark", Status: model.JobCompleted, Step: model.JobCleanup, Attempt: 1, CreatedAt: int64(100 + index), Digest: "private-digest"})
	}

	jobs = append(jobs, database.ServerJob{ID: "old-pending", Type: model.JobTransfer, SourceServerID: "queued-server", TargetServerID: "queued-server", SourceSparkID: "source-spark", TargetSparkID: "target-spark", Status: model.JobPending, Step: model.JobPreparing, Attempt: 1, CreatedAt: 1})
	if err := f.db.ORM().Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}

	var page struct {
		Items []struct {
			database.ServerJob
			SourceName string `json:"source_name"`
		} `json:"items"`
		Next string `json:"next"`
	}

	response := request(f.admin, "GET", "/api/v1/jobs", "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 50 || page.Next == "" {
		t.Fatal("platform history did not paginate across servers", response.Code, response.Body)
	}

	if page.Items[0].SourceName != page.Items[0].SourceServerID || strings.Contains(response.Body.String(), "private-digest") {
		t.Fatal("deleted endpoint history was lost or exposed private data", response.Body)
	}

	response = request(f.admin, "GET", "/api/v1/jobs?before="+page.Next, "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 21 || page.Next != "" {
		t.Fatal("platform history lost older jobs", response.Code, response.Body)
	}

	response = request(f.admin, "GET", "/api/v1/jobs?status=pending&type=transfer", "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].ID != "old-pending" || page.Next != "" {
		t.Fatal("platform filters were applied after pagination", response.Code, response.Body)
	}

	response = request(f.admin, "GET", "/api/v1/jobs?status=completed&type=copy", "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Items) != 35 {
		t.Fatal("combined filters returned unrelated workflows", response.Code, response.Body)
	}

	for _, query := range []string{"type=unknown", "status=unknown", "before=invalid"} {
		expectAccess(t, f.admin, "GET", "/api/v1/jobs?"+query, "", http.StatusBadRequest)
	}
}
