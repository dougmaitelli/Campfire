package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

// AlertConfig controls sustained thresholds relative to server allocations.
type AlertConfig struct {
	CPUPercent, MemoryPercent, DiskPercent float64
	Duration, Interval                     time.Duration
	Health                                 bool
}

// DefaultAlertConfig provides conservative sustained resource and health alerts.
func DefaultAlertConfig() AlertConfig {
	return AlertConfig{CPUPercent: 90, MemoryPercent: 90, DiskPercent: 90, Duration: 2 * time.Minute, Interval: 30 * time.Second, Health: true}
}

// Validate bounds worker timing and rejects non-finite resource thresholds.
func (c AlertConfig) Validate() error {
	for _, value := range []float64{c.CPUPercent, c.MemoryPercent, c.DiskPercent} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
			return fmt.Errorf("alert percentages must be between 0 and 100 (0 disables the resource)")
		}
	}

	if c.Duration < 0 || c.Duration > 24*time.Hour || c.Interval < 5*time.Second || c.Interval > time.Hour {
		return fmt.Errorf("alert duration must be 0–86400 seconds and interval 5–3600 seconds")
	}

	return nil
}

// NotificationService runs monitoring and optional delivery independently of browsers.
type NotificationService struct {
	Store      *repository.NotificationRepository
	Config     AlertConfig
	Apprise    *AppriseClient
	deployment *DeploymentService
	auth       *AuthService
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewNotificationService constructs a persistent inbox and monitoring service.
func NewNotificationService(store *repository.NotificationRepository, deployment *DeploymentService, auth *AuthService) *NotificationService {
	return &NotificationService{Store: store, Config: DefaultAlertConfig(), deployment: deployment, auth: auth}
}

// Start starts bounded monitoring and optional external delivery workers.
func (s *NotificationService) Start(ctx context.Context) error {
	if err := s.Config.Validate(); err != nil {
		return err
	}

	ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		timer := time.NewTimer(0)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				sampleCtx, cancel := context.WithTimeout(ctx, s.Config.Interval)
				if err := s.scan(sampleCtx); err != nil && ctx.Err() == nil {
					slog.Error("evaluate monitoring alerts", "error", err)
				}

				cancel()
				timer.Reset(s.Config.Interval)
			}
		}
	}()

	if s.Apprise != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()

			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			for {
				if err := s.deliver(ctx); err != nil && ctx.Err() == nil {
					slog.Error("deliver notifications", "error", err)
				}

				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}

	return nil
}

// Close waits for cancellation of monitoring and bounded HTTP requests.
func (s *NotificationService) Close() {
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
}

func notificationAccess(p Principal) repository.NotificationAccess {
	return repository.NotificationAccess{UserID: p.UserID, Admin: p.Admin, ServerView: p.tokenAllows("server.view"), SparkView: p.Allows("sparks.manage")}
}

// List returns the user's visible inbox, respecting current access and token scopes.
func (s *NotificationService) List(ctx context.Context, p Principal, before int64, unread bool) ([]repository.NotificationView, error) {
	if !p.tokenAllows("notifications.read") {
		return nil, repository.ErrAccessDenied
	}

	return s.Store.List(ctx, notificationAccess(p), before, unread)
}

// Unread returns the visible unread count for the current user.
func (s *NotificationService) Unread(ctx context.Context, p Principal) (int64, error) {
	if !p.tokenAllows("notifications.read") {
		return 0, repository.ErrAccessDenied
	}

	return s.Store.Unread(ctx, notificationAccess(p))
}

// MarkRead changes one personal receipt without acknowledging other recipients.
func (s *NotificationService) MarkRead(ctx context.Context, p Principal, id int64, read bool) error {
	if !p.tokenAllows("notifications.write") {
		return repository.ErrAccessDenied
	}

	return s.Store.MarkRead(ctx, notificationAccess(p), id, read, time.Now())
}

// MarkAllRead acknowledges visible messages through the supplied inbox watermark.
func (s *NotificationService) MarkAllRead(ctx context.Context, p Principal, through int64) error {
	if !p.tokenAllows("notifications.write") {
		return repository.ErrAccessDenied
	}

	return s.Store.MarkAllRead(ctx, notificationAccess(p), through, time.Now())
}

// Test creates a real inbox message and queues optional external delivery.
func (s *NotificationService) Test(ctx context.Context, p Principal) (int64, error) {
	if !p.Allows("notifications.manage") {
		return 0, repository.ErrAccessDenied
	}

	item := database.Notification{TargetType: "account", TargetID: strconv.FormatInt(p.UserID, 10), Kind: "test", Severity: "info", Title: "Test notification", Body: "Campfire notifications are working. This message also tests Apprise when external delivery is configured.", CreatedAt: time.Now().Unix()}
	err := s.Store.Emit(ctx, &item, s.Apprise != nil)

	return item.ID, err
}

func (s *NotificationService) deliver(ctx context.Context) error {
	items, err := s.Store.DueDeliveries(ctx, time.Now())
	if err != nil {
		return err
	}

	for _, item := range items {
		message := ""
		if err := s.Apprise.Send(ctx, item.Notification); err != nil {
			message = err.Error()
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := s.Store.FinishDelivery(ctx, item, message, time.Now()); err != nil {
			return err
		}
	}

	return nil
}

// transitionAlert requires continuous observations and persists active episodes.
func transitionAlert(state *database.AlertState, condition int, now int64, hold, gap int64) string {
	if condition < 0 || now < state.LastObservedAt || now-state.LastObservedAt > gap {
		state.Since = 0
		state.RecoverySince = 0
	}

	state.LastObservedAt = now

	if condition < 0 {
		return ""
	}

	if condition > 0 {
		state.RecoverySince = 0
		if state.Active {
			return ""
		}

		if state.Since == 0 {
			state.Since = now
		}

		if now-state.Since >= hold {
			state.Active = true
			state.Since = 0

			return "alert"
		}
	} else {
		state.Since = 0
		if !state.Active {
			return ""
		}

		if state.RecoverySince == 0 {
			state.RecoverySince = now
		}

		if now-state.RecoverySince >= hold {
			state.Active = false
			state.RecoverySince = 0

			return "recovery"
		}
	}

	return ""
}

func (s *NotificationService) observe(ctx context.Context, targetType, id, name, code string, condition int, body string, now time.Time) error {
	return s.Store.Observe(ctx, targetType+":"+id+":"+code, s.Apprise != nil, func(state *database.AlertState) *database.Notification {
		state.TargetType, state.TargetID = targetType, id

		kind := transitionAlert(state, condition, now.Unix(), int64(s.Config.Duration.Seconds()), int64((3*s.Config.Interval + 5*time.Second).Seconds()))
		if kind == "" {
			return nil
		}

		title, severity := name+" — "+code+" alert", "warning"
		if code == "health" || code == "quota" {
			severity = "failure"
		}

		if kind == "recovery" {
			title = name + " — " + code + " recovered"
			body = "The " + code + " condition has remained healthy for the configured recovery period."
			severity = "success"
		}

		return &database.Notification{TargetType: targetType, TargetID: id, Kind: kind, Severity: severity, Title: title, Body: body, CreatedAt: now.Unix()}
	})
}

func (s *NotificationService) scan(ctx context.Context) error {
	snapshot, err := s.deployment.Monitoring(ctx, Principal{Admin: true}, s.auth.Access())
	if err != nil {
		return err
	}

	now := time.Now()

	nodes := make(map[string]model.Spark, len(snapshot.Sparks))
	for _, node := range snapshot.Sparks {
		nodes[node.ID] = node
		if node.Disabled {
			continue
		}

		if s.Config.Health {
			condition := 0
			if !node.Connected {
				condition = 1
			}

			if err := s.observe(ctx, "spark", node.ID, node.Name, "health", condition, "Spark is disconnected from Campfire.", now); err != nil {
				return err
			}
		} else if err := s.Store.ClearAlert(ctx, "spark:"+node.ID+":health"); err != nil {
			return err
		}
	}

	for _, server := range snapshot.Servers {
		node := nodes[server.SparkID]
		if node.Disabled {
			continue
		}

		supported := node.Protocol == 0 || containsCapability(node.Capabilities, protocol.CapabilityMetrics)
		usage := server.Usage
		expectedIdle := usage.State == "not_installed" || usage.State == "deleted"

		if s.Config.Health {
			condition := 0
			if !supported {
				condition = -1
			} else if !node.Connected || usage.State == "unknown" || usage.State == "dead" || usage.State == "failed" || usage.State == "error" || usage.ContainerError != "" && !expectedIdle {
				condition = 1
			}

			if err := s.observe(ctx, "server", server.ID, server.Name, "health", condition, "Server health or monitoring is unavailable. Check its Spark, Docker runtime and server status.", now); err != nil {
				return err
			}

			condition = -1
			if usage.State == "running" && usage.SampledAt > 0 && usage.SampleAgeSeconds <= 30 {
				condition = 0
				if !usage.DiskQuotaEnforced && usage.DiskQuotaError != "" {
					condition = 1
				}
			}

			if err := s.observe(ctx, "server", server.ID, server.Name, "quota", condition, "Game filesystem quota enforcement could not be verified while the server is running.", now); err != nil {
				return err
			}
		} else {
			for _, code := range []string{"health", "quota"} {
				if err := s.Store.ClearAlert(ctx, "server:"+server.ID+":"+code); err != nil {
					return err
				}
			}
		}

		cpu, memory, disk := -1.0, -1.0, -1.0

		if supported && node.Connected && usage.SampledAt > 0 && usage.SampleAgeSeconds <= 30 && usage.ContainerError == "" {
			if usage.CPUPercent != nil && server.CPULimit > 0 {
				cpu = *usage.CPUPercent / server.CPULimit
			}

			if usage.MemoryBytes != nil && server.MemoryMB > 0 {
				memory = float64(*usage.MemoryBytes) / float64(server.MemoryMB*1048576) * 100
			}
		}

		if supported && node.Connected && usage.DiskSampledAt > 0 && usage.DiskAgeSeconds <= 120 && usage.DiskError == "" && usage.DiskBytes != nil && server.DiskMB > 0 {
			disk = float64(*usage.DiskBytes) / float64(server.DiskMB*1048576) * 100
		}

		for _, resource := range []struct {
			name             string
			value, threshold float64
		}{{"CPU", cpu, s.Config.CPUPercent}, {"memory", memory, s.Config.MemoryPercent}, {"disk", disk, s.Config.DiskPercent}} {
			key := "server:" + server.ID + ":" + resource.name
			if resource.threshold == 0 {
				if err := s.Store.ClearAlert(ctx, key); err != nil {
					return err
				}

				continue
			}
			// A five-point dead band prevents flapping close to the threshold.
			condition := -1
			if resource.value >= resource.threshold {
				condition = 1
			} else if resource.value >= 0 && resource.value <= math.Max(0, resource.threshold-5) {
				condition = 0
			}

			body := fmt.Sprintf("%s usage is %.1f%% of the server's allocation (alert threshold %.1f%%).", resource.name, resource.value, resource.threshold)
			if err := s.observe(ctx, "server", server.ID, server.Name, resource.name, condition, body, now); err != nil {
				return err
			}
		}
	}

	return s.Store.PruneAlertTargets(ctx)
}

func containsCapability(items []string, item string) bool {
	for _, value := range items {
		if value == item {
			return true
		}
	}

	return false
}
