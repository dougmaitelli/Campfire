package model

// EmberUpdate is the latest check metadata for the current Ember definition.
type EmberUpdate struct {
	EmberID   string `json:"ember_id"`
	CheckedAt int64  `json:"checked_at"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}
