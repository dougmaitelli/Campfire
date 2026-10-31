package model

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestGameIdentifiersKeepTheirSpecificRules(t *testing.T) {
	fieldName := regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
	mediaID := regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	values := make([]string, 0, 19)

	values = append(values, "", "a", "A", "1", "_", "-", ".", "game.event", "TEST-id_12", "event id/1", "a/b", "é", "a\n", "a\xff")
	for _, length := range []int{63, 64, 65, 128, 129} {
		values = append(values, strings.Repeat("a", length))
	}

	for _, value := range values {
		wantField := fieldName.MatchString(value)
		if got := ValidGameEventType(value); got != wantField {
			t.Errorf("event type %q: got %v, want %v", value, got, wantField)
		}

		state := GamePublication{Version: 1, State: &GameState{Fields: map[string]any{value: "test"}}}
		if got := state.Validate() == nil; got != wantField {
			t.Errorf("state field %q: got %v, want %v", value, got, wantField)
		}

		event := GamePublication{Version: 1, Event: &GameEvent{ID: "event id/1", Type: value, OccurredAt: time.Now(), Title: "Test event"}}
		if got := event.Validate() == nil; got != wantField {
			t.Errorf("event publication %q: got %v, want %v", value, got, wantField)
		}

		media := GameMedia{ID: value, Name: "image.png", ContentType: "image/png", Size: 1}
		if got, want := media.Valid(), mediaID.MatchString(value); got != want {
			t.Errorf("media ID %q: got %v, want %v", value, got, want)
		}
	}
}

func TestBackupNamesKeepArchiveAndResourceIDRules(t *testing.T) {
	previous := regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}\.tar\.gz$`)
	for _, name := range []string{
		"backup-TEST_12.tar.gz", "_.tar.gz", "-.tar.gz", ".tar.gz", "backup.tar", "backup.tar.GZ",
		"backup.tar.gz.tar.gz", "../backup.tar.gz", "a.b.tar.gz", "backup.tar.gz\n", "backup\xff.tar.gz",
		strings.Repeat("a", 128) + ".tar.gz", strings.Repeat("a", 129) + ".tar.gz",
	} {
		if got, want := BackupName(name), previous.MatchString(name); got != want {
			t.Errorf("backup name %q: got %v, want %v", name, got, want)
		}
	}
}
