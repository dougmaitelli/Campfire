package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

type historyRuntime struct {
	calls   atomic.Int64
	failed  atomic.Bool
	started chan struct{}
	block   bool
}

func (*historyRuntime) Deploy(context.Context, string, model.Deployment) (model.ConfigurationReceipt, error) {
	return model.ConfigurationReceipt{}, nil
}
func (r *historyRuntime) Action(ctx context.Context, _ string, q model.RuntimeRequest) (model.RuntimeStatus, error) {
	r.calls.Add(1)

	if r.block {
		select {
		case r.started <- struct{}{}:
		default:
		}

		<-ctx.Done()

		return model.RuntimeStatus{}, ctx.Err()
	}

	if r.failed.Load() {
		return model.RuntimeStatus{}, ErrDeploymentUnavailable
	}

	now := time.Now().Unix()

	return model.RuntimeStatus{ServerID: q.ServerID, Metrics: &model.ResourceUsage{ServerID: q.ServerID, State: "running", CPUPercent: new(float64(125)), MemoryBytes: new(int64(1024)), DiskBytes: new(int64(4096)), SampledAt: now, DiskSampledAt: now}}, nil
}

func TestResourceCollectionSharedAcrossReadersAndPersistsFailures(t *testing.T) {
	activity, _, _, owner := activityFixture(t)
	deployment := activity.deployment
	online := &activityConnectivity{}
	online.online.Store(true)
	deployment.Sparks.connectivity = online
	runtime := &historyRuntime{}

	deployment.Remote = runtime
	if err := deployment.History.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}

	before := runtime.calls.Load()

	for range 10 {
		snapshot, err := deployment.Monitoring(t.Context(), owner)
		if err != nil || len(snapshot.Servers) != 1 || snapshot.Servers[0].Usage.CPUPercent == nil || *snapshot.Servers[0].Usage.CPUPercent != 125 {
			t.Fatal("stored snapshot missing", snapshot, err)
		}

		if _, err := deployment.Metrics(t.Context(), "game"); err != nil {
			t.Fatal(err)
		}
	}

	if runtime.calls.Load() != before {
		t.Fatal("browser reads contacted the node")
	}

	blocked := owner
	blocked.APITokenID = 1
	blocked.TokenScopes = []string{"server.operate"}

	snapshot, err := deployment.Monitoring(t.Context(), blocked)
	if err != nil || len(snapshot.Servers) != 0 || len(snapshot.Sparks) != 0 {
		t.Fatal("API token scope leaked monitoring", snapshot, err)
	}

	runtime.failed.Store(true)

	if err := deployment.History.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}

	usage, err := deployment.Metrics(t.Context(), "game")
	if err != nil || usage.CPUPercent != nil || usage.MemoryBytes != nil || usage.ContainerError == "" {
		t.Fatal("node failure carried old usage forward", usage, err)
	}
}

func TestMonitoringExposesStoredSparkTotalsWithoutServerAccess(t *testing.T) {
	activity, db, _, _ := activityFixture(t)
	deployment := activity.deployment
	online := &activityConnectivity{}
	online.online.Store(true)
	deployment.Sparks.connectivity = online
	runtime := &historyRuntime{}
	deployment.Remote = runtime

	if err := db.ORM().Omit("Spark", "Ember", "Owner").Create(&database.Server{ID: "second", Name: "Second", SparkID: "node", EmberID: "ember", Image: "alpine:3.22", Variables: map[string]string{}, Ports: []model.ServerPort{}}).Error; err != nil {
		t.Fatal(err)
	}

	admin, err := activity.principal(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}

	// Node totals must not depend on a token's ability to list individual servers.
	admin.APITokenID = 1
	admin.TokenScopes = []string{"sparks.manage"}

	read := func() model.ResourceUsage {
		t.Helper()

		before := runtime.calls.Load()

		snapshot, readErr := deployment.Monitoring(t.Context(), admin)
		if readErr != nil || len(snapshot.Sparks) != 1 || len(snapshot.Servers) != 0 || runtime.calls.Load() != before {
			t.Fatal("node totals leaked servers or triggered sampling", snapshot, readErr)
		}

		return snapshot.Sparks[0].Usage
	}
	if usage := read(); usage.CPUPercent != nil || usage.MemoryBytes != nil || usage.DiskBytes != nil {
		t.Fatal("missing samples fabricated totals", usage)
	}

	if err := deployment.History.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}

	usage := read()
	if usage.CPUPercent == nil || *usage.CPUPercent != 250 || usage.MemoryBytes == nil || *usage.MemoryBytes != 2048 || usage.DiskBytes == nil || *usage.DiskBytes != 8192 {
		t.Fatal("incorrect stored fleet totals", usage)
	}

	online.online.Store(false)

	if usage := read(); usage.CPUPercent != nil || usage.ContainerError == "" {
		t.Fatal("disconnected node exposed old totals", usage)
	}

	online.online.Store(true)
	runtime.failed.Store(true)

	if err := deployment.History.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}

	if usage := read(); usage.CPUPercent != nil || usage.MemoryBytes != nil || usage.DiskBytes != nil {
		t.Fatal("failed sampling carried previous totals forward", usage)
	}
}

func TestResourceJobsCancelInFlightCollection(t *testing.T) {
	activity, _, _, _ := activityFixture(t)
	online := &activityConnectivity{}
	online.online.Store(true)
	activity.deployment.Sparks.connectivity = online
	runtime := &historyRuntime{block: true, started: make(chan struct{}, 1)}
	activity.deployment.Remote = runtime

	history := activity.deployment.History
	if err := history.Start(t.Context()); err != nil {
		t.Fatal(err)
	}

	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		history.Close()
		t.Fatal("collector did not start without a browser")
	}

	done := make(chan struct{})

	go func() { history.Close(); close(done) }()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("history jobs did not stop")
	}
}

func TestResourceHistoryValidationAndAge(t *testing.T) {
	activity, _, _, _ := activityFixture(t)

	history := activity.deployment.History
	for _, request := range []struct {
		from, to int64
		limit    int
	}{{-1, 1, 1}, {100, 99, 1}, {1, time.Now().Unix() + 3600, 1}, {1, 2, -1}, {1, 2, 1001}} {
		if _, err := history.History(t.Context(), false, "game", request.from, request.to, request.limit); err == nil {
			t.Fatal("invalid history query accepted", request)
		}
	}

	usage := ageResourceUsage(model.ResourceUsage{SampleAgeSeconds: 2, DiskAgeSeconds: 5}, 100, 150)
	if usage.SampleAgeSeconds != 52 || usage.DiskAgeSeconds != 55 {
		t.Fatal("persisted sample age reset on read", usage)
	}
}
