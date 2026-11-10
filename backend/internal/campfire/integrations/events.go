package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"campfire/internal/shared/model"
)

// Event is the provider-independent snapshot handed to outgoing integrations.
// Source IDs are namespaced so different producers cannot collide in a binding.
type Event struct {
	ID          string                `json:"id"`
	ServerID    string                `json:"server_id"`
	Category    Category              `json:"category"`
	Type        string                `json:"type"`
	Title       string                `json:"title"`
	Body        string                `json:"body,omitempty"`
	OccurredAt  int64                 `json:"occurred_at"`
	Data        map[string]any        `json:"data"`
	FieldLabels model.GameFieldLabels `json:"field_labels,omitempty"`
}

// Validate excludes unbounded or structured data from notification payloads.
func (e Event) Validate() error {
	if !validID(e.ID) || !validID(e.ServerID) || !e.Category.Valid() || !model.ValidGameEventType(e.Type) || e.OccurredAt < -62135596800 || e.OccurredAt > 253402300799 || e.Title == "" || len(e.Title) > 1024 || len(e.Body) > 8192 || !utf8.ValidString(e.Title+e.Body) || len(e.Data) > 32 {
		return errors.New("invalid integration event")
	}

	for key, value := range e.Data {
		if !model.ValidGameEventType(key) {
			return errors.New("invalid integration event field")
		}

		if _, ok := ScalarText(value); !ok {
			return errors.New("invalid integration event value")
		}
	}

	return e.FieldLabels.Validate(e.Data)
}

// ScalarText matches Activity's field dropdown representation, including false
// and zero. Empty filter values mean field presence, not an empty-string match.
func ScalarText(value any) (string, bool) {
	if text, ok := value.(string); ok {
		return text, utf8.ValidString(text) && len(text) <= 8192
	}

	switch value.(type) {
	case bool, float64, int, int64:
		encoded, err := json.Marshal(value)
		return string(encoded), err == nil
	default:
		return "", false
	}
}

// Matches ORs rule groups; each group ANDs its type and field conditions.
func (s Subscription) Matches(e Event) bool {
	if s.Category != e.Category {
		return false
	}

	if len(s.Rules) == 0 {
		return true
	}

	for _, rule := range s.Rules {
		if (rule.Type == "" || rule.Type == e.Type) && rule.matchesFields(e.Data) {
			return true
		}
	}

	return false
}

func (r EventRule) matchesFields(data map[string]any) bool {
	for field, expected := range r.Fields {
		value, exists := data[field]

		actual, scalar := ScalarText(value)
		if !exists || !scalar || expected != "" && !strings.EqualFold(actual, expected) {
			return false
		}
	}

	return true
}

// GameEvent keeps mod identity and data without attaching media bytes or URLs.
func GameEvent(serverID string, event model.GameEvent) Event {
	id := sha256.Sum256([]byte(event.ID))

	data := event.Data
	if data == nil {
		data = map[string]any{}
	}

	return Event{ID: "game:" + hex.EncodeToString(id[:]), ServerID: serverID, Category: GameEvents, Type: event.Type, Title: event.Title, OccurredAt: event.OccurredAt.Unix(), Data: data, FieldLabels: event.FieldLabels}
}

// ConsoleAlert deliberately omits evidence, paths, and raw console output.
func ConsoleAlert(serverID string, alert model.DetectedLogAlert) Event {
	return Event{ID: "console_alert:" + alert.ID, ServerID: serverID, Category: ConsoleAlerts, Type: "console.alert", Title: alert.Title, Body: alert.Message, OccurredAt: alert.DetectedAt, Data: map[string]any{"severity": string(alert.Severity), "phase": string(alert.Phase)}}
}

// OperationResult never equates dispatch acceptance with successful completion.
// Unconfirmed and skipped operations retain their explicit result state.
func OperationResult(serverID, id string, action model.ServerAction, state model.ExecutionState, backupMode model.BackupMode, now time.Time) (Event, bool) {
	if !slices.Contains([]model.ExecutionState{model.ExecutionSucceeded, model.ExecutionFailed, model.ExecutionInterrupted, model.ExecutionSkipped}, state) {
		return Event{}, false
	}

	category := PowerEvents

	switch action {
	case model.ServerActionBackup, model.ServerActionRestore:
		category = BackupEvents
	case model.ServerActionStart, model.ServerActionStop, model.ServerActionRestart, model.ServerActionKill, model.ServerActionInstall, model.ServerActionUpdate, model.ServerActionReinstall:
	default:
		return Event{}, false
	}

	data := map[string]any{"action": string(action), "state": string(state), "operation_id": id}
	if action == model.ServerActionBackup {
		data["backup_mode"] = string(backupMode.Normalized())
	}

	return Event{ID: "operation:" + id, ServerID: serverID, Category: category, Type: string(action) + "." + string(state), Title: "Server " + string(action) + " " + string(state), OccurredAt: now.Unix(), Data: data}, true
}
