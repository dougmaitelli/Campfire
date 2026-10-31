package model

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"campfire/internal/shared/utils"
)

// Game media is transferred separately from the bounded JSON control messages.
const (
	GameMediaFileLimit   = 25 << 20
	GameMediaServerLimit = 250 << 20
	GameMediaTotalLimit  = 1 << 30
	GameMediaEventLimit  = 4
)

var (
	// ErrGameMediaInvalid rejects unsafe metadata or unsupported bytes.
	ErrGameMediaInvalid = errors.New("invalid media metadata or content")
	// ErrGameMediaLimit covers file, count, and storage budgets.
	ErrGameMediaLimit = errors.New("media file, attachment count, or storage limit exceeded")
	// ErrGameMediaConflict preserves the first committed attachment for an ID.
	ErrGameMediaConflict = errors.New("attachment ID already has different content")
	// ErrGameMediaEventMissing allows retry while the event queue catches up.
	ErrGameMediaEventMissing = errors.New("event is not retained yet; publish the event first and retry")
	// ErrGameMediaPlacement rejects a former or unrelated server owner.
	ErrGameMediaPlacement = errors.New("media source does not own this server")
	// ErrGameMediaNotFound hides missing or differently scoped attachments.
	ErrGameMediaNotFound = errors.New("media not found")
	// ErrGameMediaBusy bounds simultaneous uploads.
	ErrGameMediaBusy = errors.New("media uploads busy; retry later")
)

// GameMedia describes one immutable attachment scoped to an event.
type GameMedia struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// Valid checks metadata without trusting a producer-supplied filename or MIME type.
func (m GameMedia) Valid() bool {
	if len(m.ID) > 64 || !utils.IsValidResourceID(m.ID) || len(m.Name) < 1 || len(m.Name) > 128 || !utf8.ValidString(m.Name) || strings.ContainsAny(m.Name, "/\\") || strings.ContainsFunc(m.Name, unicode.IsControl) {
		return false
	}

	switch m.ContentType {
	case "image/png", "image/jpeg", "video/mp4", "video/webm":
		return m.Size >= 0 && m.Size <= GameMediaFileLimit
	default:
		return false
	}
}
