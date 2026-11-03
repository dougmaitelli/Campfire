package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// LogAlert describes an actionable console message. Identity comes from its
// matching conditions, independently of its title, guidance and list position.
type LogAlert struct {
	Title    string          `json:"title"`
	Message  string          `json:"message"`
	Severity LogSeverity     `json:"severity"`
	Phase    LogPhase        `json:"phase,omitempty"`
	Any      []LogMatch      `json:"any"`
	Action   *LogAlertAction `json:"action,omitempty"`
}

// UnmarshalJSON moves the earlier rule-wide flag onto its match conditions.
// New definitions serialize only per-condition flags.
func (a *LogAlert) UnmarshalJSON(data []byte) error {
	type fields LogAlert

	decoded := struct {
		*fields
		IgnoreCase bool `json:"ignore_case"`
		StripANSI  bool `json:"strip_ansi"`
	}{fields: (*fields)(a)}
	if err := decodeLogSettings(data, &decoded); err != nil {
		return err
	}

	if decoded.IgnoreCase {
		for index := range a.Any {
			a.Any[index].IgnoreCase = true
		}
	}

	return nil
}

func decodeLogSettings(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	return decoder.Decode(target)
}

// LogSeverity is distinct from process state and notification delivery state.
type LogSeverity string

// Log alert severities describe attention independently of lifecycle state.
const (
	LogWarning LogSeverity = "warning"
	LogError   LogSeverity = "error"
)

// LogPhase selects game output, installer output, or both (the default).
type LogPhase string

// Output sources have separate execution identities and matching cursors.
const (
	LogRuntime      LogPhase = "runtime"
	LogInstallation LogPhase = "installation"
)

// LogMatch uses the same literal and RE2 matching semantics as readiness.
type LogMatch struct {
	Value      string `json:"value"`
	Regex      bool   `json:"regex,omitempty"`
	IgnoreCase bool   `json:"ignore_case,omitempty"`
}

// LogActionType permits navigation to existing permission-checked interfaces.
type LogActionType string

// Navigation targets share the panel's existing permission checks.
const (
	LogOpenFile       LogActionType = "open_file"
	LogServerSettings LogActionType = "server_settings"
)

// LogAlertAction never executes commands or changes server configuration.
type LogAlertAction struct {
	Type LogActionType `json:"type"`
	Path string        `json:"path,omitempty"`
}

// Validate rejects unsupported navigation actions and unsafe file paths.
func (a LogAlertAction) Validate() error {
	if a.Type == LogOpenFile && SafePath(a.Path) && len(a.Path) <= 512 && !strings.ContainsFunc(a.Path, unicode.IsControl) {
		return nil
	}

	if a.Type == LogServerSettings && a.Path == "" {
		return nil
	}

	return errors.New("log alert action must open a relative file or server settings")
}

// Validate bounds metadata and validates all matching conditions.
func (a LogAlert) Validate() error {
	if strings.TrimSpace(a.Title) == "" || len(a.Title) > 120 || strings.ContainsFunc(a.Title, unicode.IsControl) || strings.TrimSpace(a.Message) == "" || len(a.Message) > 2048 || strings.ContainsRune(a.Message, 0) {
		return errors.New("log alerts require a title of 1–120 bytes and guidance of 1–2048 bytes")
	}

	if a.Severity != LogWarning && a.Severity != LogError || a.Phase != "" && a.Phase != LogRuntime && a.Phase != LogInstallation {
		return errors.New("invalid log alert severity or phase")
	}

	if len(a.Any) == 0 || len(a.Any) > 16 {
		return errors.New("log alerts require 1–16 match conditions")
	}

	for _, match := range a.Any {
		if len(match.Value) == 0 || len(match.Value) > 512 || strings.ContainsAny(match.Value, "\x00\r\n") {
			return errors.New("log alert match conditions require 1–512 bytes on a single line")
		}

		if match.Regex {
			if _, err := regexp.Compile(match.Value); err != nil {
				return errors.New("invalid log alert regular expression")
			}
		}
	}

	if a.Action != nil {
		return a.Action.Validate()
	}

	return nil
}

// Key canonicalizes conditions so reordering a rule or its OR alternatives does
// not repeat a dismissed issue. Changing matching behavior creates a new key.
func (a LogAlert) Key() string {
	// Preserve existing keys when every condition uses the same case behavior.
	ignoreCase := len(a.Any) > 0
	for _, match := range a.Any {
		ignoreCase = ignoreCase && match.IgnoreCase
	}

	conditions := make([]string, 0, len(a.Any))
	for _, match := range a.Any {
		if match.IgnoreCase && !match.Regex {
			match.Value = strings.ToLower(match.Value)
		}

		if ignoreCase {
			match.IgnoreCase = false
		}

		data, _ := json.Marshal(match)
		conditions = append(conditions, string(data))
	}

	slices.Sort(conditions)
	conditions = slices.Compact(conditions)
	data, _ := json.Marshal([]any{a.Phase, true, ignoreCase, conditions})
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:])
}

// DetectedLogAlert is a bounded observation persisted by Spark and Campfire.
type DetectedLogAlert struct {
	ID          string          `json:"id"`
	RuleKey     string          `json:"rule_key"`
	ExecutionID string          `json:"execution_id"`
	Phase       LogPhase        `json:"phase"`
	Title       string          `json:"title"`
	Message     string          `json:"message"`
	Severity    LogSeverity     `json:"severity"`
	Action      *LogAlertAction `json:"action,omitempty"`
	DetectedAt  int64           `json:"detected_at"`
	Line        string          `json:"line,omitempty"`
}

// LogObservation carries recent detections and current execution identities.
// The capped history preserves errors across disconnected server restarts.
type LogObservation struct {
	Executions map[LogPhase]string `json:"executions"`
	Alerts     []DetectedLogAlert  `json:"alerts"`
	Error      string              `json:"error,omitempty"`
}

// LogObservationBytes leaves room for the rest of the bounded protocol frame.
const LogObservationBytes = 512 << 10

// LogAlertHistoryLimit caps the replayable detection history on each Spark.
const LogAlertHistoryLimit = 256

// RetainLogAlerts caps history by count and encoded size, preserving new entries.
func RetainLogAlerts(items []DetectedLogAlert) []DetectedLogAlert {
	if len(items) > LogAlertHistoryLimit {
		items = items[len(items)-LogAlertHistoryLimit:]
	}

	for len(items) > 0 {
		encoded, _ := json.Marshal(items)
		if len(encoded) < LogObservationBytes-4096 {
			break
		}

		items = items[1:]
	}

	return items
}

// Validate checks the bounded observation at the authenticated Spark boundary.
func (o LogObservation) Validate() error {
	data, err := json.Marshal(o)
	if err != nil || len(data) > LogObservationBytes {
		return errors.New("log observations exceed the byte limit")
	}

	if len(o.Executions) > 2 || len(o.Alerts) > LogAlertHistoryLimit || len(o.Error) > 512 {
		return errors.New("log observations exceed supported limits")
	}

	for phase, id := range o.Executions {
		if (phase != LogRuntime && phase != LogInstallation) || !logDigest(id) {
			return errors.New("invalid log execution identity")
		}
	}

	seen := make(map[string]bool)
	for _, alert := range o.Alerts {
		if !logDigest(alert.ID) || !logDigest(alert.RuleKey) || !logDigest(alert.ExecutionID) || alert.DetectedAt <= 0 || len(alert.Line) > 1024 || seen[alert.ID] {
			return errors.New("invalid log alert observation")
		}

		seen[alert.ID] = true

		rule := LogAlert{Title: alert.Title, Message: alert.Message, Severity: alert.Severity, Phase: alert.Phase, Any: []LogMatch{{Value: "observed"}}, Action: alert.Action}
		if err := rule.Validate(); err != nil || alert.Phase == "" {
			return errors.New("invalid observed log alert")
		}
	}

	return nil
}

func logDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
