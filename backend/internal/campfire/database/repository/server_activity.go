package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrActivityInput distinguishes malformed history queries from storage failures.
var ErrActivityInput = errors.New("invalid activity filter or cursor")

// ServerActivityAccess is resolved from current grants before any query, count,
// source lookup or pagination. It never grants mutation rights by itself.
type ServerActivityAccess struct {
	Console, Backups, ManualOperations, ScheduledOperations bool
	ManageIntegrations, ManageNotifications                 bool
	JobTypes                                                []model.JobType
	Categories                                              []integrations.Category
}

// ServerActivityFilter keeps source-specific filters explicit in the shared feed.
type ServerActivityFilter struct {
	Kind, Search, State, Before string
	From, To                    int64
	GameType                    string
	Fields                      map[string]string
}

// Validate bounds queries independently of UI controls and producer data.
func (f ServerActivityFilter) Validate() error {
	if !slices.Contains([]string{"", "game", "operations", "alert"}, f.Kind) || len(f.Search) > 1024 || !utf8.ValidString(f.Search) || strings.ContainsRune(f.Search, 0) || f.From < 0 || f.To < 0 || f.From > 253402300799 || f.To > 253402300799 || f.From > 0 && f.To > 0 && f.From >= f.To {
		return ErrActivityInput
	}

	if !slices.Contains([]string{"", "active", "dismissed", "archived", "raised", "recovered", "pending", "running", "dispatching", "failed", "completed", "cancelled", "cancelling", "succeeded", "interrupted", "skipped", "delivered", "uncertain", "created", "dispatch_failed"}, f.State) || !(GameEventQuery{Type: f.GameType, Fields: f.Fields}).ValidFilters() {
		return ErrActivityInput
	}

	return nil
}

// ServerActivityEntry is a read projection, not a second persisted event log.
type ServerActivityEntry struct {
	Key                   string `json:"key"`
	Kind                  string `json:"kind"`
	SourceKey             string `json:"source_key,omitempty"`
	SourceAvailable       bool   `json:"source_available"`
	CreatedAt             int64  `json:"created_at"`
	Title                 string `json:"title"`
	State                 string `json:"state"`
	DeliveryCheckPending  bool   `json:"delivery_check_pending,omitempty"`
	Provider              string `json:"provider,omitempty"`
	Destination           string `json:"destination,omitempty"`
	AttachmentName        string `json:"attachment_name,omitempty"`
	AttachmentContentType string `json:"attachment_content_type,omitempty"`
	NotificationCount     int64  `json:"notification_count"`
}

// ServerActivityPage uses a stable timestamp/key cursor across heterogeneous IDs.
type ServerActivityPage struct {
	Items []ServerActivityEntry `json:"items"`
	Next  string                `json:"next"`
}

type serverActivityCursor struct {
	At  int64  `json:"at"`
	Key string `json:"key"`
}

// ServerActivityRepository queries the existing domain stores without altering retention.
type ServerActivityRepository struct {
	db          *gorm.DB
	choicesMu   sync.Mutex
	gameChoices map[string]activityGameChoices
}

// NewServerActivityRepository constructs the server-scoped history read model.
func NewServerActivityRepository(db *gorm.DB) *ServerActivityRepository {
	return &ServerActivityRepository{db: db, gameChoices: make(map[string]activityGameChoices)}
}

func (r *ServerActivityRepository) jsonText(column string, fields ...string) string {
	if r.db.Name() == "postgres" {
		return "COALESCE(CAST(" + column + " AS JSONB)#>>'{" + strings.Join(fields, ",") + "}', '')"
	}

	return "COALESCE(json_extract(" + column + ", '$." + strings.Join(fields, ".") + "'), '')"
}

func activityOperations(query *gorm.DB, access ServerActivityAccess) *gorm.DB {
	query = query.Where("((schedule_id = '' AND ?) OR (schedule_id <> '' AND ?))", access.ManualOperations, access.ScheduledOperations)
	if !access.Backups {
		query = query.Where("action NOT IN ?", []model.ServerAction{model.ServerActionBackup, model.ServerActionRestore})
	}

	return query
}

// sourceExists uses primary-key lookups, including a guarded numeric cast, rather
// than comparing computed keys across entire history tables.
func (r *ServerActivityRepository) sourceExists(serverID, reference string, access ServerActivityAccess) clause.Expr {
	parts := make([]string, 0, 4)
	parts = append(parts,
		"("+reference+" LIKE 'game_event:%' AND EXISTS (SELECT 1 FROM game_events source WHERE source.server_id = ? AND source.id = CASE WHEN "+reference+" LIKE 'game_event:%' THEN CAST(substr("+reference+", 12) AS BIGINT) ELSE NULL END))",
		"("+reference+" LIKE 'console_alert:%' AND EXISTS (SELECT 1 FROM log_alert_issues source WHERE source.server_id = ? AND source.id = substr("+reference+", 15)))",
		"("+reference+" LIKE 'monitoring_alert:%' AND EXISTS (SELECT 1 FROM monitoring_alerts source WHERE source.target_type = ? AND source.target_id = ? AND source.id = substr("+reference+", 18)))",
	)
	args := make([]any, 0, 6)
	args = append(args, serverID, serverID, model.NotificationServer, serverID)
	operations := activityOperations(r.db.Model(&database.ScheduleRun{}).Select("1").Where("server_id = ? AND id = substr("+reference+", 11)", serverID), access)
	parts = append(parts, "("+reference+" LIKE 'operation:%' AND EXISTS (?))")
	args = append(args, operations)
	parts = append(parts, "("+reference+" LIKE 'job:%' AND EXISTS (SELECT 1 FROM server_jobs source WHERE (source.source_server_id = ? OR source.target_server_id = ?) AND source.type IN ? AND source.id = substr("+reference+", 5)))")
	args = append(args, serverID, serverID, access.JobTypes)

	return clause.Expr{SQL: "(" + strings.Join(parts, " OR ") + ")", Vars: args}
}

type activityRoots struct {
	game, alerts, operations, jobs *gorm.DB
	monitoringAlerts               *gorm.DB
}

func (q activityRoots) all() []*gorm.DB {
	return []*gorm.DB{q.game, q.alerts, q.operations, q.jobs, q.monitoringAlerts}
}

func (r *ServerActivityRepository) roots(ctx context.Context, serverID string, access ServerActivityAccess) activityRoots {
	db := r.db.WithContext(ctx)
	game := db.Model(&database.GameEvent{}).Select("'game_event:' || CAST(id AS TEXT) AS key, 'game' AS kind, '' AS source_key, received_at AS created_at, title, '' AS state, '' AS provider, '' AS destination, title AS search_text, FALSE AS delivery_check_pending, '' AS attachment_name, '' AS attachment_content_type").Where("server_id = ?", serverID)
	alertTitle, alertMessage := r.jsonText("detection", "title"), r.jsonText("detection", "message")
	alerts := db.Model(&database.LogAlertIssue{}).Select("'console_alert:' || id AS key, 'alert' AS kind, '' AS source_key, received_at AS created_at, "+alertTitle+" AS title, CASE WHEN archived THEN 'archived' WHEN dismissed_at > 0 THEN 'dismissed' ELSE 'active' END AS state, '' AS provider, '' AS destination, "+alertTitle+" || ' ' || "+alertMessage+" AS search_text, FALSE AS delivery_check_pending, '' AS attachment_name, '' AS attachment_content_type").Where("server_id = ?", serverID)
	operations := activityOperations(db.Model(&database.ScheduleRun{}).Select("'operation:' || id AS key, 'operation' AS kind, '' AS source_key, created_at, action AS title, state, '' AS provider, '' AS destination, action || ' ' || message || ' ' || username AS search_text, FALSE AS delivery_check_pending, '' AS attachment_name, '' AS attachment_content_type").Where("server_id = ?", serverID), access)
	jobs := db.Model(&database.ServerJob{}).Select("'job:' || id AS key, 'job' AS kind, '' AS source_key, created_at, type AS title, status AS state, '' AS provider, '' AS destination, type || ' ' || id AS search_text, FALSE AS delivery_check_pending, '' AS attachment_name, '' AS attachment_content_type").Where("(source_server_id = ? OR target_server_id = ?) AND type IN ?", serverID, serverID, access.JobTypes)
	monitoringAlerts := db.Model(&database.MonitoringAlert{}).Select("'monitoring_alert:' || id AS key, 'alert' AS kind, '' AS source_key, created_at, title, CASE WHEN kind = 'recovery' THEN 'recovered' ELSE 'raised' END AS state, '' AS provider, '' AS destination, title || ' ' || body AS search_text, FALSE AS delivery_check_pending, '' AS attachment_name, '' AS attachment_content_type").Where("target_type = ? AND target_id = ?", model.NotificationServer, serverID)

	return activityRoots{game: game, alerts: alerts, operations: operations, jobs: jobs, monitoringAlerts: monitoringAlerts}
}

type activityNotifications struct {
	panel, outgoing *gorm.DB
}

func (q activityNotifications) all() []*gorm.DB {
	return []*gorm.DB{q.panel, q.outgoing}
}

func (r *ServerActivityRepository) notifications(ctx context.Context, serverID string, access ServerActivityAccess) activityNotifications {
	db := r.db.WithContext(ctx)
	panel := db.Model(&database.Notification{}).Select("'notification:' || CAST(notifications.id AS TEXT) AS key, 'notification' AS kind, notifications.source_key, notifications.created_at, notifications.title, COALESCE(d.state, 'created') AS state, CASE WHEN d.notification_id IS NULL THEN 'campfire' ELSE 'apprise' END AS provider, '' AS destination, notifications.title || ' ' || notifications.body AS search_text, FALSE AS delivery_check_pending, '' AS attachment_name, '' AS attachment_content_type").Joins("LEFT JOIN notification_deliveries d ON d.notification_id = notifications.id").Where("notifications.target_type = ? AND notifications.target_id = ?", model.NotificationServer, serverID)
	state := "CASE WHEN m.state = ? THEN 'delivered' WHEN m.state IN ? THEN 'failed' WHEN m.state IN ? THEN 'uncertain' ELSE 'pending' END"
	source := "m.source_key"
	title, body := r.jsonText("m.payload", "title"), r.jsonText("m.payload", "body")
	attachmentName := r.jsonText("m.payload", "attachment", "media", "name")
	attachmentContentType := r.jsonText("m.payload", "attachment", "media", "content_type")
	uncertainStates := integrations.UncertainStates()
	// A worker can finish a claimed check after its integration is disabled.
	pendingCheck := "((m.state = ? AND m.next_attempt_at > ?) OR (b.enabled AND c.enabled AND m.next_attempt_at > 0 AND m.state IN ?)) AS delivery_check_pending"
	outgoing := db.Table("integration_messages m").Select("'integration_message:' || CAST(m.id AS TEXT) AS key, 'notification' AS kind, "+source+" AS source_key, m.created_at, "+title+" AS title, "+state+" AS state, c.provider, m.destination_id AS destination, "+title+" || ' ' || "+body+" AS search_text, "+pendingCheck+", "+attachmentName+" AS attachment_name, "+attachmentContentType+" AS attachment_content_type", model.DeliveryDelivered, integrations.FailureStates(), uncertainStates, integrations.MessageReconciling, time.Now().Unix(), uncertainStates).Joins("JOIN server_integrations b ON b.id = m.binding_id").Joins("JOIN integrations c ON c.id = b.integration_id").Where("b.server_id = ? AND m.category IN ? AND m.state <> ?", serverID, access.Categories, integrations.MessageReceipt)

	return activityNotifications{panel: panel, outgoing: outgoing}
}

func activityUnion(db *gorm.DB, queries []*gorm.DB) *gorm.DB {
	parts, args := make([]string, len(queries)), make([]any, len(queries))
	for index, query := range queries {
		parts[index], args[index] = "?", query
	}

	return db.Table("(?) AS activity", db.Raw(strings.Join(parts, " UNION ALL "), args...))
}

// List groups notifications under retained, accessible sources in All.
func (r *ServerActivityRepository) List(ctx context.Context, serverID string, access ServerActivityAccess, filter ServerActivityFilter) (ServerActivityPage, error) {
	page := ServerActivityPage{Items: []ServerActivityEntry{}, Next: ""}
	if err := filter.Validate(); err != nil {
		return page, err
	}

	roots := r.roots(ctx, serverID, access)
	if filter.Search != "" {
		roots.game = searchGameEvents(roots.game, filter.Search)
	}

	if filter.GameType != "" {
		roots.game = roots.game.Where("type = ?", filter.GameType)
	}

	for field, value := range filter.Fields {
		patterns := eventFieldPatterns(field, value)

		condition := r.db.Where("LOWER(data) LIKE ? ESCAPE '!'", patterns[0])
		for _, pattern := range patterns[1:] {
			condition = condition.Or("LOWER(data) LIKE ? ESCAPE '!'", pattern)
		}

		roots.game = roots.game.Where(condition)
	}

	queries := append(roots.all(), r.notifications(ctx, serverID, access).all()...)
	query := activityUnion(r.db.WithContext(ctx), queries)

	if filter.Kind != "" {
		kinds := []string{filter.Kind}
		if filter.Kind == "operations" {
			kinds = []string{"operation", "job"}
		}

		query = query.Where("kind IN ?", kinds)
	} else {
		query = query.Where("kind <> 'notification' OR NOT ?", r.sourceExists(serverID, "activity.source_key", access))
	}

	if filter.GameType != "" || len(filter.Fields) > 0 {
		query = query.Where("kind = 'game'")
	}

	if filter.Search != "" {
		// Game rows were filtered above using the JSON serializer's encoding.
		query = query.Where("kind = 'game' OR LOWER(search_text) LIKE ? ESCAPE '!'", eventLikePattern(strings.ToLower(filter.Search)))
	}

	if filter.State != "" {
		query = query.Where("state = ?", filter.State)
	}

	if filter.From > 0 {
		query = query.Where("created_at >= ?", filter.From)
	}

	if filter.To > 0 {
		query = query.Where("created_at < ?", filter.To)
	}

	return r.readPage(ctx, serverID, access, query, filter.Before)
}

// Notifications pages delivery outcomes for one retained, accessible source.
func (r *ServerActivityRepository) Notifications(ctx context.Context, serverID, key string, access ServerActivityAccess, before string) (ServerActivityPage, error) {
	entry, err := r.entry(ctx, serverID, key, access)
	if err != nil {
		return ServerActivityPage{}, err
	}

	if entry.Kind == "notification" {
		return ServerActivityPage{}, ErrAccessNotFound
	}

	query := activityUnion(r.db.WithContext(ctx), r.notifications(ctx, serverID, access).all()).Where("source_key = ?", key)

	return r.readPage(ctx, serverID, access, query, before)
}

func (r *ServerActivityRepository) readPage(ctx context.Context, serverID string, access ServerActivityAccess, query *gorm.DB, before string) (ServerActivityPage, error) {
	page := ServerActivityPage{Items: []ServerActivityEntry{}, Next: ""}

	if before != "" {
		var cursor serverActivityCursor

		if len(before) > 1024 {
			return page, ErrActivityInput
		}

		encoded, err := base64.RawURLEncoding.DecodeString(before)
		if err != nil || len(encoded) > 512 || json.Unmarshal(encoded, &cursor) != nil || cursor.At < 0 || cursor.At > 253402300799 || cursor.Key == "" || len(cursor.Key) > 256 || !utf8.ValidString(cursor.Key) || strings.ContainsRune(cursor.Key, 0) {
			return page, ErrActivityInput
		}

		query = query.Where("created_at < ? OR (created_at = ? AND key < ?)", cursor.At, cursor.At, cursor.Key)
	}

	if err := query.Select("key, kind, source_key, created_at, title, state, provider, destination, delivery_check_pending, attachment_name, attachment_content_type").Order("created_at DESC, key DESC").Limit(51).Scan(&page.Items).Error; err != nil {
		return page, err
	}

	if len(page.Items) > 50 {
		page.Items = page.Items[:50]
		last := page.Items[49]
		encoded, _ := json.Marshal(serverActivityCursor{At: last.CreatedAt, Key: last.Key})
		page.Next = base64.RawURLEncoding.EncodeToString(encoded)
	}

	return page, r.enrichEntries(ctx, serverID, access, page.Items)
}

func (r *ServerActivityRepository) enrichEntries(ctx context.Context, serverID string, access ServerActivityAccess, items []ServerActivityEntry) error {
	keys, sources := make([]string, 0, len(items)), make([]string, 0, len(items))
	for _, item := range items {
		keys = append(keys, item.Key)
		if item.SourceKey != "" {
			sources = append(sources, item.SourceKey)
		}
	}

	if len(items) == 0 {
		return nil
	}

	var counts []struct {
		SourceKey string
		Count     int64
	}
	if err := activityUnion(r.db.WithContext(ctx), r.notifications(ctx, serverID, access).all()).Select("source_key, COUNT(*) AS count").Where("source_key IN ?", keys).Group("source_key").Scan(&counts).Error; err != nil {
		return err
	}

	bySource := map[string]int64{}
	for _, count := range counts {
		bySource[count.SourceKey] = count.Count
	}

	var available []ServerActivityEntry

	if len(sources) > 0 {
		queries := r.rootsByKeys(ctx, serverID, access, sources)
		if len(queries) > 0 {
			if err := activityUnion(r.db.WithContext(ctx), queries).Select("key").Scan(&available).Error; err != nil {
				return err
			}
		}
	}

	for index := range items {
		items[index].NotificationCount = bySource[items[index].Key]
		items[index].SourceAvailable = slices.ContainsFunc(available, func(source ServerActivityEntry) bool { return source.Key == items[index].SourceKey })
	}

	return nil
}
