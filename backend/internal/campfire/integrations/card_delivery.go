package integrations

import "campfire/internal/shared/model"

// CardCategory is reserved for singleton control messages, never subscriptions.
// Card work reuses the outbox table but is excluded from notification delivery.
const CardCategory Category = "server_card"

// Card phases use existing outbox columns, without changing the wire protocol.
const (
	CardEventID                     = "card:singleton"
	CardReady   model.DeliveryState = "card_ready"
	CardClaimed model.DeliveryState = "card_claimed"
	CardSending model.DeliveryState = "card_sending"
)

// CardWork is the versioned, provider-neutral payload of a card control row.
// Creating remains true until an ambiguous creation has been reconciled.
type CardWork struct {
	Version    int    `json:"version"`
	CreationID string `json:"creation_id"`
	Creating   bool   `json:"creating"`
	CreatedAt  int64  `json:"created_at"`
	Hash       string `json:"hash"`
	CheckedAt  int64  `json:"checked_at"`
}

// CardStatus omits external IDs and internal creation/lease identities.
type CardStatus struct {
	State         string `json:"state"`
	Error         string `json:"error"`
	NextAttemptAt int64  `json:"next_attempt_at"`
	CanRetry      bool   `json:"can_retry"`
}
