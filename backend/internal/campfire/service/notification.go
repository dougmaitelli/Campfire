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
	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"
	"campfire/internal/shared/protocol"
)

// AlertConfig controls sustained thresholds relative to server allocations.
type AlertConfig struct {
	CPUPercent, MemoryPercent, DiskPercent float64
	Duration                               time.Duration
	Health                                 bool
}

// DefaultAlertConfig provides conservative sustained resource and health alerts.
func DefaultAlertConfig() AlertConfig {
	return AlertConfig{CPUPercent: limits.DefaultAlertPercent, MemoryPercent: limits.DefaultAlertPercent, DiskPercent: limits.DefaultAlertPercent, Duration: limits.DefaultAlertDuration, Health: limits.DefaultAlertHealth}
}

// Validate bounds worker timing and rejects non-finite resource thresholds.
func (c AlertConfig) Validate() error {
	for _, value := range []float64{c.CPUPercent, c.MemoryPercent, c.DiskPercent} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
			return fmt.Errorf("alert percentages must be between 0 and 100 (0 disables the resource)")
		}
	}

	if c.Duration < 0 || c.Duration > limits.MaxAlertDuration {
		return fmt.Errorf("alert duration must be 0–86400 seconds")
	}

	return nil
}

// NotificationService evaluates collected resources and delivers notifications.
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

// Start validates alert settings and starts the optional external delivery worker.
func (s *NotificationService) Start(ctx context.Context) error {
	if err := s.Config.Validate(); err != nil {
		return err
	}

	ctx, s.cancel = context.WithCancel(ctx)

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
	return repository.NotificationAccess{UserID: p.UserID, Admin: p.Admin, ServerView: p.tokenAllows(accessperm.ServerView), SparkView: p.Allows(accessperm.SparksManage)}
}

// List returns the user's visible inbox, respecting current access and token scopes.
func (s *NotificationService) List(ctx context.Context, p Principal, before int64, unread bool) ([]repository.NotificationView, error) {
	if !p.tokenAllows(accessperm.NotificationsRead) {
		return nil, repository.ErrAccessDenied
	}

	return s.Store.List(ctx, notificationAccess(p), before, unread)
}

// Unread returns the visible unread count for the current user.
func (s *NotificationService) Unread(ctx context.Context, p Principal) (int64, error) {
	if !p.tokenAllows(accessperm.NotificationsRead) {
		return 0, repository.ErrAccessDenied
	}

	return s.Store.Unread(ctx, notificationAccess(p))
}

// MarkRead changes one personal receipt without acknowledging other recipients.
func (s *NotificationService) MarkRead(ctx context.Context, p Principal, id int64, read bool) error {
	if !p.tokenAllows(accessperm.NotificationsWrite) {
		return repository.ErrAccessDenied
	}

	return s.Store.MarkRead(ctx, notificationAccess(p), id, read, time.Now())
}

// MarkAllRead acknowledges visible messages through the supplied inbox watermark.
func (s *NotificationService) MarkAllRead(ctx context.Context, p Principal, through int64) error {
	if !p.tokenAllows(accessperm.NotificationsWrite) {
		return repository.ErrAccessDenied
	}

	return s.Store.MarkAllRead(ctx, notificationAccess(p), through, time.Now())
}

// Test creates a real inbox message and queues optional external delivery.
func (s *NotificationService) Test(ctx context.Context, p Principal) (int64, error) {
	if !p.Allows(accessperm.NotificationsManage) {
		return 0, repository.ErrAccessDenied
	}

	item := database.Notification{TargetType: model.NotificationAccount, TargetID: strconv.FormatInt(p.UserID, 10), Kind: model.NotificationTest, Severity: model.NotificationInfo, Title: "Test notification", Body: "Campfire notifications are working. This message also tests Apprise when external delivery is configured.", CreatedAt: time.Now().Unix()}
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
func transitionAlert(state *database.AlertState, condition model.AlertCondition, now int64, hold, gap int64) model.NotificationKind {
	if condition == model.AlertUnknown || now < state.LastObservedAt || now-state.LastObservedAt > gap {
		state.Since = 0
		state.RecoverySince = 0
	}

	state.LastObservedAt = now

	if condition == model.AlertUnknown {
		return ""
	}

	if condition == model.AlertUnhealthy {
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

			return model.NotificationAlert
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

			return model.NotificationRecovery
		}
	}

	return ""
}

type alertObservation struct {
	TargetType           model.NotificationTarget
	TargetID, Name, Code string
	Condition            model.AlertCondition
	Body                 string
	At                   time.Time
}

func (s *NotificationService) observe(ctx context.Context, observation alertObservation) error {
	key := fmt.Sprintf("%s:%s:%s", observation.TargetType, observation.TargetID, observation.Code)

	return s.Store.Observe(ctx, key, s.Apprise != nil, func(state *database.AlertState) *database.Notification {
		state.TargetType, state.TargetID = observation.TargetType, observation.TargetID

		kind := transitionAlert(state, observation.Condition, observation.At.Unix(), int64(s.Config.Duration.Seconds()), int64((3*s.deployment.History.Interval + resourceSampleTimeout).Seconds()))
		if kind == "" {
			return nil
		}

		title := fmt.Sprintf("%s — %s alert", observation.Name, observation.Code)

		severity := model.NotificationWarning
		if observation.Code == "health" || observation.Code == "quota" {
			severity = model.NotificationFailure
		}

		if kind == model.NotificationRecovery {
			title = fmt.Sprintf("%s — %s recovered", observation.Name, observation.Code)
			observation.Body = "The " + observation.Code + " condition has remained healthy for the configured recovery period."
			severity = model.NotificationSuccess
		}

		return &database.Notification{TargetType: observation.TargetType, TargetID: observation.TargetID, Kind: kind, Severity: severity, Title: title, Body: observation.Body, CreatedAt: observation.At.Unix()}
	})
}

func (s *NotificationService) scan(ctx context.Context) error {
	snapshot, err := s.deployment.Monitoring(ctx, Principal{Admin: true})
	if err != nil {
		return err
	}

	now := time.Now()
	freshFor := max(int64(30), 3*snapshot.SampleIntervalSeconds)

	nodes := make(map[string]model.Spark, len(snapshot.Sparks))
	for _, node := range snapshot.Sparks {
		nodes[node.ID] = node.Spark
		if node.Disabled {
			continue
		}

		if s.Config.Health {
			condition := model.AlertHealthy
			if !node.Connected {
				condition = model.AlertUnhealthy
			}

			if err := s.observe(ctx, alertObservation{TargetType: model.NotificationSpark, TargetID: node.ID, Name: node.Name, Code: "health", Condition: condition, Body: "Spark is disconnected from Campfire.", At: now}); err != nil {
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
		expectedIdle := usage.State == model.RuntimeStateNotInstalled || usage.State == model.RuntimeStateDeleted

		if s.Config.Health {
			condition := model.AlertHealthy
			if !supported {
				condition = model.AlertUnknown
			} else if !node.Connected || usage.State == model.RuntimeStateUnknown || usage.State == model.RuntimeStateDead || usage.State == "failed" || usage.State == "error" || usage.ContainerError != "" && !expectedIdle {
				condition = model.AlertUnhealthy
			} else if !expectedIdle && (usage.SampledAt == 0 || usage.SampleAgeSeconds > freshFor) {
				condition = model.AlertUnknown
			}

			if err := s.observe(ctx, alertObservation{TargetType: model.NotificationServer, TargetID: server.ID, Name: server.Name, Code: "health", Condition: condition, Body: "Server health or monitoring is unavailable. Check its Spark, Docker runtime and server status.", At: now}); err != nil {
				return err
			}

			condition = model.AlertUnknown
			if usage.State == model.RuntimeStateRunning && usage.SampledAt > 0 && usage.SampleAgeSeconds <= freshFor {
				condition = model.AlertHealthy
				if !usage.DiskQuotaEnforced && usage.DiskQuotaError != "" {
					condition = model.AlertUnhealthy
				}
			}

			if err := s.observe(ctx, alertObservation{TargetType: model.NotificationServer, TargetID: server.ID, Name: server.Name, Code: "quota", Condition: condition, Body: "Game filesystem quota enforcement could not be verified while the server is running.", At: now}); err != nil {
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

		if supported && node.Connected && usage.SampledAt > 0 && usage.SampleAgeSeconds <= freshFor && usage.ContainerError == "" {
			if usage.CPUPercent != nil && server.CPULimit > 0 {
				cpu = *usage.CPUPercent / server.CPULimit
			}

			if usage.MemoryBytes != nil && server.MemoryMB > 0 {
				memory = float64(*usage.MemoryBytes) / float64(server.MemoryMB*1048576) * 100
			}
		}

		if supported && node.Connected && usage.DiskSampledAt > 0 && usage.DiskAgeSeconds <= max(int64(120), freshFor) && usage.DiskError == "" && usage.DiskBytes != nil && server.DiskMB > 0 {
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
			condition := model.AlertUnknown
			if resource.value >= resource.threshold {
				condition = model.AlertUnhealthy
			} else if resource.value >= 0 && resource.value <= math.Max(0, resource.threshold-5) {
				condition = model.AlertHealthy
			}

			body := fmt.Sprintf("%s usage is %.1f%% of the server's allocation (alert threshold %.1f%%).", resource.name, resource.value, resource.threshold)
			if err := s.observe(ctx, alertObservation{TargetType: model.NotificationServer, TargetID: server.ID, Name: server.Name, Code: resource.name, Condition: condition, Body: body, At: now}); err != nil {
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
