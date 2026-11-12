package repository

import (
	"encoding/json"
	"errors"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func saveGameEventLabels(tx *gorm.DB, serverID, eventType string, labels model.GameFieldLabels) error {
	if len(labels) == 0 {
		return nil
	}

	// Registered publishers own an explicit schema. Legacy inline labels cannot
	// mutate it; label-free events never execute this lookup or any label writes.
	registered, err := hasGameRegistration(tx, serverID)
	if err != nil || registered {
		return err
	}

	rows := make([]database.GameEventFieldLabel, 0, len(labels))
	for key, label := range labels {
		rows = append(rows, database.GameEventFieldLabel{ServerID: serverID, EventType: eventType, FieldKey: key, Label: label})
	}

	return tx.Omit("Server").Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "server_id"}, {Name: "event_type"}, {Name: "field_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"label"}),
	}).Create(&rows).Error
}

func readGameEventLabels(tx *gorm.DB, serverID string, types ...string) (map[string]model.GameFieldLabels, error) {
	query := tx.Where("server_id = ?", serverID)
	if len(types) > 0 {
		query = query.Where("event_type IN ?", types)
	}

	var rows []database.GameEventFieldLabel
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}

	labels := map[string]model.GameFieldLabels{}
	for _, row := range rows {
		if labels[row.EventType] == nil {
			labels[row.EventType] = model.GameFieldLabels{}
		}

		labels[row.EventType][row.FieldKey] = row.Label
	}

	return labels, nil
}

func labelsForGameFields(labels model.GameFieldLabels, fields map[string]any) model.GameFieldLabels {
	selected := model.GameFieldLabels{}

	for key := range fields {
		if label, exists := labels[key]; exists {
			selected[key] = label
		}
	}

	return selected
}

// visitGameEventData streams retained scalar maps on either supported database.
func visitGameEventData(tx *gorm.DB, serverID string, visit func(string, map[string]any) error) error {
	rows, err := tx.Model(&database.GameEvent{}).Select("type", "data").Where("server_id = ?", serverID).Rows()
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var eventType string

		var raw []byte
		if err := rows.Scan(&eventType, &raw); err != nil {
			return err
		}

		var fields map[string]any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &fields); err != nil {
				return err
			}
		}

		if err := visit(eventType, fields); err != nil {
			return err
		}
	}

	return rows.Err()
}

// Startup/hourly retention cleans definitions no longer referenced by history.
// Ingestion avoids scanning all retained history for each count-based eviction.
// Filter choices independently include only fields still present in history.
func pruneGameEventLabels(tx *gorm.DB, serverID string) error {
	// Coordinate cleanup with manifest replacement, including registration
	// arriving while the first retained event is being pruned.
	var source database.Server
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("id").Take(&source, "id = ?", serverID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}

		return err
	}

	registered, err := hasGameRegistration(tx, serverID)
	if err != nil || registered {
		return err
	}

	labels, err := readGameEventLabels(tx, serverID)
	if err != nil || len(labels) == 0 {
		return err
	}

	if err := visitGameEventData(tx, serverID, func(eventType string, fields map[string]any) error {
		for key := range fields {
			delete(labels[eventType], key)
		}

		return nil
	}); err != nil {
		return err
	}

	for eventType, fields := range labels {
		for key := range fields {
			if err := tx.Where("server_id = ? AND event_type = ? AND field_key = ?", serverID, eventType, key).Delete(&database.GameEventFieldLabel{}).Error; err != nil {
				return err
			}
		}
	}

	return nil
}
