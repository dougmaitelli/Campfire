package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

type notificationRuntime struct {
	usage model.ResourceUsage
	fail  bool
}

func (*notificationRuntime) Deploy(context.Context, string, model.Deployment) (model.ConfigurationReceipt, error) {
	return model.ConfigurationReceipt{}, nil
}
func (r *notificationRuntime) Action(_ context.Context, _ string, q model.RuntimeRequest) (model.RuntimeStatus, error) {
	if r.fail {
		return model.RuntimeStatus{}, errors.New("unavailable")
	}

	return model.RuntimeStatus{ServerID: q.ServerID, Metrics: &r.usage}, nil
}

func TestAlertTransitionsAreSustainedAndUnknownDoesNotRecover(t *testing.T) {
	state := database.AlertState{}
	for _, now := range []int64{100, 160} {
		if result := transitionAlert(&state, 1, now, 120, 100); result != "" {
			t.Fatal("alert did not wait", result)
		}
	}

	if result := transitionAlert(&state, 1, 220, 120, 100); result != "alert" {
		t.Fatal("sustained alert missing", result)
	}

	if result := transitionAlert(&state, 1, 230, 120, 100); result != "" {
		t.Fatal("active alert repeated")
	}

	if result := transitionAlert(&state, -1, 240, 120, 100); result != "" || !state.Active {
		t.Fatal("unknown sample recovered alert")
	}

	if result := transitionAlert(&state, 0, 250, 120, 100); result != "" {
		t.Fatal("recovery did not wait")
	}

	if result := transitionAlert(&state, 0, 450, 120, 100); result != "" {
		t.Fatal("observation gap counted toward recovery")
	}

	if result := transitionAlert(&state, 0, 510, 120, 100); result != "" {
		t.Fatal(result)
	}

	if result := transitionAlert(&state, 0, 570, 120, 100); result != "recovery" {
		t.Fatal("recovery missing", result)
	}
}

func TestMonitoringCreatesPersistentNotificationsWithoutApprise(t *testing.T) {
	activity, db, _, owner := activityFixture(t)
	deployment := activity.deployment
	online := &activityConnectivity{online: true}
	deployment.Sparks.connectivity = online

	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "game").Update("cpu_limit", 2).Error; err != nil {
		t.Fatal(err)
	}

	runtime := &notificationRuntime{usage: model.ResourceUsage{ServerID: "game", State: "running", CPUPercent: new(float64(190)), MemoryBytes: new(int64(120 * 1048576)), DiskBytes: new(int64(950 * 1048576)), SampledAt: time.Now().Unix(), DiskSampledAt: time.Now().Unix(), DiskQuotaEnforced: true}}
	deployment.Remote = runtime
	service := NewNotificationService(repository.NewNotificationRepository(db.ORM()), deployment, activity.auth)
	service.Config.Duration = 0

	ctx := context.Background()
	if err := service.scan(ctx); err != nil {
		t.Fatal(err)
	}

	messages, err := service.List(ctx, owner, 0, false)
	if err != nil || len(messages) != 3 {
		t.Fatal("resource alerts missing", len(messages), err)
	}

	foundCPU := false

	for _, item := range messages {
		if strings.Contains(item.Body, "CPU usage is 95.0%") {
			foundCPU = true
		}
	}

	if !foundCPU {
		t.Fatal("CPU was not normalized to its two-core allocation")
	}

	restarted := NewNotificationService(service.Store, deployment, activity.auth)

	restarted.Config = service.Config
	if err := restarted.scan(ctx); err != nil {
		t.Fatal(err)
	}

	messages, err = restarted.List(ctx, owner, 0, false)
	if err != nil || len(messages) != 3 {
		t.Fatal("restart duplicated active alerts", len(messages), err)
	}

	runtime.usage.CPUPercent = nil
	runtime.usage.MemoryBytes = nil
	runtime.usage.DiskBytes = nil

	if err := restarted.scan(ctx); err != nil {
		t.Fatal(err)
	}

	messages, err = restarted.List(ctx, owner, 0, false)
	if err != nil || len(messages) != 3 {
		t.Fatal("missing samples caused false recovery", len(messages), err)
	}

	runtime.usage.CPUPercent = new(float64(50))
	runtime.usage.MemoryBytes = new(int64(1))
	runtime.usage.DiskBytes = new(int64(1))

	if err := restarted.scan(ctx); err != nil {
		t.Fatal(err)
	}

	messages, err = restarted.List(ctx, owner, 0, false)
	if err != nil || len(messages) != 6 {
		t.Fatal("recovery messages missing", len(messages), err)
	}

	online.online = false

	if err := restarted.scan(ctx); err != nil {
		t.Fatal(err)
	}

	messages, err = restarted.List(ctx, owner, 0, false)
	if err != nil || len(messages) != 7 {
		t.Fatal("server availability alert missing or Spark information leaked", len(messages), err)
	}

	adminMessages, err := restarted.List(ctx, Principal{UserID: 1, Admin: true}, 0, false)
	if err != nil || len(adminMessages) != 8 {
		t.Fatal("administrator did not receive Spark and server alerts", len(adminMessages), err)
	}

	pending, failed, err := service.Store.DeliveryCounts(ctx)
	if err != nil || pending != 0 || failed != 0 {
		t.Fatal("disabled Apprise queued work", pending, failed, err)
	}
}

func TestAppriseDeliveryIsIndependentAndDoesNotLeakCredentials(t *testing.T) {
	var payload struct {
		Title, Body, Type string
		URLs              []string `json:"urls"`
		Tags              []string `json:"tag"`
	}

	reject := true

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("unexpected Apprise request")
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}

		if reject {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("secret response body"))

			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer host.Close()

	client, err := NewAppriseClient(AppriseConfig{URL: host.URL + "/notify/private-key?token=private", URLs: []string{"discord://private-target"}, Tags: []string{"admins"}})
	if err != nil {
		t.Fatal(err)
	}

	activity, db, _, _ := activityFixture(t)
	service := NewNotificationService(repository.NewNotificationRepository(db.ORM()), activity.deployment, activity.auth)
	service.Apprise = client
	ctx := context.Background()

	id, err := service.Test(ctx, Principal{UserID: 1, Admin: true})
	if err != nil {
		t.Fatal(err)
	}

	if err := service.deliver(ctx); err != nil {
		t.Fatal(err)
	}

	var delivery database.NotificationDelivery
	if err := db.ORM().Take(&delivery, "notification_id = ?", id).Error; err != nil {
		t.Fatal(err)
	}

	if delivery.Attempts != 1 || delivery.State != "pending" || delivery.Error != "apprise returned HTTP 503" || delivery.NextAttemptAt <= time.Now().Unix() {
		t.Fatal("retry not persisted safely", delivery.Error)
	}

	messages, err := service.List(ctx, Principal{UserID: 1, Admin: true}, 0, false)
	if err != nil || len(messages) != 1 || messages[0].ReadAt != 0 {
		t.Fatal("external failure changed inbox", err)
	}

	reject = false

	if err := db.ORM().Model(&delivery).Update("next_attempt_at", 0).Error; err != nil {
		t.Fatal(err)
	}

	if err := service.deliver(ctx); err != nil {
		t.Fatal(err)
	}

	if err := db.ORM().Take(&delivery, "notification_id = ?", id).Error; err != nil {
		t.Fatal(err)
	}

	if delivery.State != "delivered" || delivery.Attempts != 2 || delivery.Error != "" || delivery.DeliveredAt == 0 {
		t.Fatal("delivery result missing", delivery.State)
	}

	if payload.Title != "Campfire — Test notification" || payload.Type != "info" || len(payload.URLs) != 1 || len(payload.Tags) != 1 {
		t.Fatal("DockDash Apprise contract differs")
	}

	redirects := 0

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { redirects++; w.WriteHeader(http.StatusOK) }))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	redirectClient, err := NewAppriseClient(AppriseConfig{URL: redirect.URL})
	if err != nil {
		t.Fatal(err)
	}

	if err := redirectClient.Send(ctx, database.Notification{}); err == nil || redirects != 0 {
		t.Fatal("notification credentials followed a redirect")
	}
}
