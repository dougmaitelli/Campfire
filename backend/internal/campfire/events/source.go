package events

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// GameEventSourceKey links text and subsequent media deliveries to one retained event.
func GameEventSourceKey(id int64) string { return "game_event:" + strconv.FormatInt(id, 10) }

// ValidSourceKey bounds soft references independently of producer event IDs.
// References survive source retention and never affect delivery deduplication.
func ValidSourceKey(key string) bool {
	if key == "" {
		return true
	}

	kind, id, ok := strings.Cut(key, ":")
	if !ok || id == "" || len(key) > 256 || !utf8.ValidString(key) || strings.ContainsRune(key, 0) {
		return false
	}

	switch kind {
	case "game_event":
		number, err := strconv.ParseInt(id, 10, 64)
		return err == nil && number > 0 && strconv.FormatInt(number, 10) == id
	case "console_alert", "monitoring_alert", "operation", "job":
		return true
	default:
		return false
	}
}
