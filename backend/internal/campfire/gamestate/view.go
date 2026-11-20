// Package gamestate defines the panel's timestamped integration-state view.
package gamestate

import "campfire/internal/shared/model"

// View records when Campfire received the latest snapshot. State is omitted
// when runtime or placement checks invalidate it; consumers decide its age.
type View struct {
	State     *model.GameState `json:"state,omitempty"`
	UpdatedAt int64            `json:"updated_at"`
}
