package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func TestMonitoringFiltersBeforeSamplingAndRevokesAccess(t *testing.T) {
	f := newAccessFixture(t)
	user, member := f.user("monitor", database.UserRole)
	_, _, input := serverFixture(t, f.admin)

	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	f.runtime.metrics = func(q model.RuntimeRequest) (model.RuntimeStatus, error) {
		if q.Action != "metrics" || q.OperationID != "" {
			t.Fatal("metrics became a lifecycle operation", q)
		}

		return model.RuntimeStatus{ServerID: q.ServerID, Metrics: &model.ResourceUsage{ServerID: q.ServerID, State: "running", CPUPercent: new(float64(125)), MemoryBytes: new(int64(1024)), DiskBytes: new(int64(4096)), SampledAt: 1, DiskSampledAt: 1}}, nil
	}
	before := f.runtime.calls.Load()

	expectAccess(t, member, "GET", endpoint+"/metrics", "", http.StatusNotFound)
	expectAccess(t, member, "GET", "/api/v1/monitoring", "", http.StatusOK)

	if f.runtime.calls.Load() != before {
		t.Fatal("inaccessible server was sampled")
	}

	expectAccess(t, f.admin, "POST", endpoint+"/members", `{"username":"monitor","role_id":5}`, http.StatusOK)

	response = request(member, "GET", "/api/v1/monitoring", "")

	var snapshot service.MonitoringSnapshot

	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}

	if len(snapshot.Servers) != 1 || len(snapshot.Sparks) != 0 || snapshot.Servers[0].Usage.CPUPercent == nil || *snapshot.Servers[0].Usage.CPUPercent != 125 {
		t.Fatal("incorrect authorized monitoring snapshot", response.Body)
	}

	expectAccess(t, member, "GET", endpoint+"/metrics", "", http.StatusOK)
	expectAccess(t, f.admin, "DELETE", endpoint+"/members/"+fmt.Sprint(user.ID), "", http.StatusNoContent)
	before = f.runtime.calls.Load()

	expectAccess(t, member, "GET", endpoint+"/metrics", "", http.StatusNotFound)
	expectAccess(t, member, "GET", "/api/v1/monitoring", "", http.StatusOK)

	if f.runtime.calls.Load() != before {
		t.Fatal("revoked member reached Spark")
	}

	f.runtime.metrics = nil

	response = request(f.admin, "GET", "/api/v1/monitoring", "")
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}

	if response.Code != http.StatusOK || len(snapshot.Servers) != 1 || len(snapshot.Sparks) != 1 || snapshot.Servers[0].Usage.CPUPercent != nil || snapshot.Servers[0].Usage.ContainerError == "" {
		t.Fatal("unsupported Spark fabricated usage", response.Body)
	}

	expectAccess(t, f.admin, "GET", endpoint+"/metrics", "", http.StatusServiceUnavailable)
}
