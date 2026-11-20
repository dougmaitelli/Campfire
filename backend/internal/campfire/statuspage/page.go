// Package statuspage defines the panel-owned public availability contract.
package statuspage

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"campfire/internal/campfire/gamestate"
	"campfire/internal/shared/utils"
)

// Publication bounds keep anonymous responses and management lists bounded.
const (
	MaxEntries  = 50
	MaxPages    = 100
	PollSeconds = 10
)

// Publication errors contain only safe, actionable metadata.
var (
	ErrInput    = errors.New("provide a title, a unique lowercase URL slug (3–64 letters, digits or hyphens), and up to 50 distinct servers with public names")
	ErrNotFound = errors.New("status page not found")
	ErrConflict = errors.New("status page URL is already in use")
	ErrLimit    = errors.New("at most 100 status pages are allowed")
)
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

// ValidSlug permits one unambiguous lowercase URL segment.
func ValidSlug(slug string) bool { return slugPattern.MatchString(slug) }

// Selection is an administrator's explicit choice of server and public name.
type Selection struct {
	ServerID string `json:"server_id"`
	Name     string `json:"name"`
}

// Input contains editable publication metadata; entry order is significant.
type Input struct {
	Title   string      `json:"title"`
	Slug    string      `json:"slug"`
	Entries []Selection `json:"entries"`
}

// Page is the authenticated management response.
type Page struct {
	ID string `json:"id"`
	Input
}

// Normalize validates only publication metadata, not server configuration.
func (p *Input) Normalize() error {
	p.Title = strings.TrimSpace(p.Title)
	if !validName(p.Title) || !ValidSlug(p.Slug) || len(p.Entries) > MaxEntries {
		return ErrInput
	}

	seen := make(map[string]bool, len(p.Entries))
	for i := range p.Entries {
		entry := &p.Entries[i]

		entry.Name = strings.TrimSpace(entry.Name)
		if !utils.IsValidResourceID(entry.ServerID) || !validName(entry.Name) || seen[entry.ServerID] {
			return ErrInput
		}

		seen[entry.ServerID] = true
	}

	return nil
}
func validName(s string) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) > 0 && utf8.RuneCountInString(s) <= 100 && strings.IndexFunc(s, unicode.IsControl) == -1
}

// State describes availability without exposing private lifecycle details.
type State string

// Public availability states do not imply an external network connectivity test.
const (
	Online  State = "online"
	Offline State = "offline"
	Unknown State = "unknown"
)

// PublicEntry exposes selected game information, never private deployment details.
type PublicEntry struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	State       State           `json:"state"`
	ArtURL      string          `json:"art_url,omitempty"`
	GameAddress string          `json:"game_address,omitempty"`
	GameState   *gamestate.View `json:"game_state,omitempty"`
	ObservedAt  int64           `json:"observed_at"`
}

// PublicPage is the complete anonymous response, independent of private models.
type PublicPage struct {
	// CollectedAt anchors browser elapsed time to Campfire's clock, not the visitor's.
	CollectedAt int64         `json:"collected_at"`
	Title       string        `json:"title"`
	Entries     []PublicEntry `json:"entries"`
	PollSeconds int           `json:"poll_seconds"`
}
