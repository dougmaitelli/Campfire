package repository

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/events"
	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrGameEventPlacement rejects ingestion from a deleted or reassigned server.
var ErrGameEventPlacement = errors.New("game event source does not own server")

// GameEventQuery uses a receipt-order cursor independent of game clocks.
type GameEventQuery struct {
	Before int64
	From   int64
	To     int64
	Type   string
	Search string
	Fields map[string]string
	Limit  int
}

// ValidFilters bounds user-supplied searches and mod-defined field conditions.
func (q GameEventQuery) ValidFilters() bool {
	if q.Type != "" && !model.ValidGameEventType(q.Type) || len(q.Search) > limits.GameFieldStringLength || !utf8.ValidString(q.Search) || strings.ContainsRune(q.Search, 0) || len(q.Fields) > limits.GameFields {
		return false
	}

	for field, value := range q.Fields {
		if !model.ValidGameEventType(field) || len(value) > limits.GameFieldStringLength || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return false
		}
	}

	return true
}

// GameEventPage contains a bounded page and its next receipt-order cursor.
type GameEventPage struct {
	Items      []database.GameEvent `json:"items"`
	NextBefore int64                `json:"next_before,omitempty"`
}

// GameEventFilterOptions contains choices from a server's entire retained history.
type GameEventFilterOptions struct {
	Types       []string                         `json:"types"`
	Fields      map[string][]string              `json:"fields"`
	FieldLabels map[string]model.GameFieldLabels `json:"field_labels,omitempty"`
}

// GameEventRepository owns durable deduplication and bounded retained history.
type GameEventRepository struct {
	db        *gorm.DB
	publisher *EventPublisher
}

// NewGameEventRepository uses the panel's shared transactional database.
func NewGameEventRepository(db *gorm.DB, publishers ...*EventPublisher) *GameEventRepository {
	return &GameEventRepository{db: db, publisher: eventPublisher(db, publishers)}
}

// FilterOptions streams only the columns needed for dropdown choices, independently
// of the displayed page or active filters.
func (r *GameEventRepository) FilterOptions(ctx context.Context, serverID string) (GameEventFilterOptions, error) {
	options := GameEventFilterOptions{Types: []string{}, Fields: map[string][]string{}, FieldLabels: map[string]model.GameFieldLabels{}}

	types := map[string]struct{}{}
	fields := map[string]map[string]struct{}{}

	err := visitGameEventData(r.db.WithContext(ctx), serverID, func(eventType string, data map[string]any) error {
		types[eventType] = struct{}{}
		if options.FieldLabels[eventType] == nil {
			options.FieldLabels[eventType] = model.GameFieldLabels{}
		}

		for field, value := range data {
			options.FieldLabels[eventType][field] = field

			if fields[field] == nil {
				fields[field] = map[string]struct{}{}
			}

			text, ok := value.(string)
			if !ok {
				encoded, err := json.Marshal(value)
				if err != nil {
					return err
				}

				text = string(encoded)
			}

			if text != "" {
				fields[field][text] = struct{}{}
			}
		}

		return nil
	})
	if err != nil {
		return options, err
	}

	labels, err := readGameEventLabels(r.db.WithContext(ctx), serverID)
	if err != nil {
		return options, err
	}

	for eventType, entries := range options.FieldLabels {
		for key := range entries {
			entries[key] = labels[eventType].Label(key)
		}
	}

	for eventType := range types {
		options.Types = append(options.Types, eventType)
	}

	slices.Sort(options.Types)

	for field, values := range fields {
		options.Fields[field] = []string{}
		for value := range values {
			options.Fields[field] = append(options.Fields[field], value)
		}

		slices.Sort(options.Fields[field])
	}

	return options, nil
}

// Append commits the event, source check, and retention changes together.
func (r *GameEventRepository) Append(ctx context.Context, sparkID, serverID string, event model.GameEvent, now time.Time, retention time.Duration, maximum int) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&database.Server{}).Where("id = ? AND spark_id = ? AND game_integration = ?", serverID, sparkID, true).Count(&count).Error; err != nil {
			return err
		}

		if count == 0 {
			return ErrGameEventPlacement
		}

		row := database.GameEvent{ServerID: serverID, SparkID: sparkID, EventID: event.ID, Type: event.Type, Title: event.Title, OccurredAt: event.OccurredAt.UTC(), ReceivedAt: now.Unix(), Data: event.Data}
		if row.Data == nil {
			row.Data = map[string]any{}
		}

		result := tx.Omit("Server").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "server_id"}, {Name: "event_id"}}, DoNothing: true}).Create(&row)
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected > 0 {
			if err := saveGameEventLabels(tx, serverID, event.Type, event.FieldLabels); err != nil {
				return err
			}

			labels, err := readGameEventLabels(tx, serverID, event.Type)
			if err != nil {
				return err
			}

			event.FieldLabels = labelsForGameFields(labels[event.Type], event.Data)
			if _, err := r.publisher.inTransaction(tx).Publish(ctx, events.GameEventRecorded{ServerID: serverID, RecordID: row.ID, Event: event}, now); err != nil {
				return err
			}
		}

		return pruneGameServer(tx, serverID, now.Add(-retention).Unix(), maximum)
	})
}

func pruneGameServer(tx *gorm.DB, serverID string, before int64, maximum int) error {
	if err := tx.Where("server_id = ? AND received_at < ?", serverID, before).Delete(&database.GameEvent{}).Error; err != nil {
		return err
	}

	newest := tx.Model(&database.GameEvent{}).Select("id").Where("server_id = ?", serverID).Order("id DESC").Limit(maximum)

	return tx.Where("server_id = ? AND id NOT IN (?)", serverID, newest).Delete(&database.GameEvent{}).Error
}

// List returns receipt-ordered events; optional time filters use occurrence time.
func (r *GameEventRepository) List(ctx context.Context, serverID string, input GameEventQuery) (GameEventPage, error) {
	page := GameEventPage{Items: []database.GameEvent{}}
	if input.Limit < 1 || input.Limit > 100 {
		return page, errors.New("invalid game event page limit")
	}

	if !input.ValidFilters() {
		return page, errors.New("invalid game event filter")
	}

	query := r.db.WithContext(ctx).Preload("Attachments", func(db *gorm.DB) *gorm.DB { return db.Omit("Data").Order("id") }).Where("server_id = ?", serverID)
	if input.Before > 0 {
		query = query.Where("id < ?", input.Before)
	}

	if input.Type != "" {
		query = query.Where("type = ?", input.Type)
	}

	if input.Search != "" {
		query = searchGameEvents(query, input.Search)
	}

	for field, value := range input.Fields {
		patterns := eventFieldPatterns(field, value)
		condition := r.db.Where("LOWER(data) LIKE ? ESCAPE '!'", patterns[0])

		for _, pattern := range patterns[1:] {
			condition = condition.Or("LOWER(data) LIKE ? ESCAPE '!'", pattern)
		}

		query = query.Where(condition)
	}

	if input.From > 0 {
		query = query.Where("occurred_at >= ?", time.Unix(input.From, 0).UTC())
	}

	if input.To > 0 {
		query = query.Where("occurred_at < ?", time.Unix(input.To, 0).UTC())
	}

	if err := query.Order("id DESC").Limit(input.Limit + 1).Find(&page.Items).Error; err != nil {
		return page, err
	}

	if len(page.Items) > input.Limit {
		page.Items = page.Items[:input.Limit]
		page.NextBefore = page.Items[len(page.Items)-1].ID
	}

	if len(page.Items) > 0 {
		types := make([]string, 0, len(page.Items))
		for _, event := range page.Items {
			types = append(types, event.Type)
		}

		labels, err := readGameEventLabels(r.db.WithContext(ctx), serverID, types...)
		if err != nil {
			return page, err
		}

		for i := range page.Items {
			event := &page.Items[i]
			event.FieldLabels = labelsForGameFields(labels[event.Type], event.Data)
		}
	}

	return page, nil
}

// Game data uses JSON escaping while titles remain plain text. Both history
// views share these patterns so searches match the values displayed to users.
func searchGameEvents(query *gorm.DB, search string) *gorm.DB {
	encoded, _ := json.Marshal(strings.ToLower(search))
	textPattern := eventLikePattern(strings.ToLower(search))
	dataPattern := eventLikePattern(string(encoded[1 : len(encoded)-1]))

	return query.Where("(LOWER(title) LIKE ? ESCAPE '!' OR LOWER(data) LIKE ? ESCAPE '!')", textPattern, dataPattern)
}

func eventLikePattern(value string) string {
	return "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(value) + "%"
}

// Data uses the compact encoding/json serializer on both supported databases.
// Complete key/value fragments distinguish field values from text in other fields.
func eventFieldPatterns(field, value string) []string {
	key, _ := json.Marshal(field)
	prefix := string(key) + ":"

	if value == "" {
		return []string{eventLikePattern(prefix)}
	}

	value = strings.ToLower(value)
	quoted, _ := json.Marshal(value)
	patterns := []string{eventLikePattern(prefix + string(quoted))}

	var scalar any

	if json.Unmarshal([]byte(value), &scalar) == nil {
		switch scalar.(type) {
		case bool, float64:
			encoded, _ := json.Marshal(scalar)
			patterns = append(patterns, eventLikePattern(prefix+string(encoded)+","), eventLikePattern(prefix+string(encoded)+"}"))
		}
	}

	return patterns
}

// Prune also enforces reduced limits and expiration when no new events arrive.
func (r *GameEventRepository) Prune(ctx context.Context, before int64, maximum int) error {
	var ids []string
	if err := r.db.WithContext(ctx).Model(&database.GameEvent{}).Distinct().Pluck("server_id", &ids).Error; err != nil {
		return err
	}

	for _, id := range ids {
		if err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
			if err := pruneGameServer(tx, id, before, maximum); err != nil {
				return err
			}

			return pruneGameEventLabels(tx, id)
		}); err != nil {
			return err
		}
	}

	return nil
}
