package model

// EmberUpdate is mutable check metadata kept outside immutable definitions.
type EmberUpdate struct {
	EmberID   string `json:"ember_id"`
	URL       string `json:"url"`
	CheckedAt int64  `json:"checked_at"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}
