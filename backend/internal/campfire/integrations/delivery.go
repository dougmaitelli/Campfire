package integrations

import (
	"time"

	"campfire/internal/shared/model"
)

// Delivery phases are panel-local values in the existing message state column.
// Claimed work is safe to retry; a lost sending lease requires reconciliation.
const (
	// MessageReceipt retains only the event identity after history expires.
	MessageReceipt            model.DeliveryState = "receipt"
	MessageClaimed            model.DeliveryState = "claimed"
	MessageSending            model.DeliveryState = "sending"
	MessageReconciling        model.DeliveryState = "reconciling"
	MessageUncertain          model.DeliveryState = "uncertain"
	MessageRateLimited        model.DeliveryState = "rate_limited"
	MessageGlobalLimited      model.DeliveryState = "global_rate_limited"
	MessageGlobalUncertain    model.DeliveryState = "global_rate_limited_uncertain"
	MessageCredentialsBlocked model.DeliveryState = "credentials_blocked"
	MessageDestinationBlocked model.DeliveryState = "destination_blocked"
	DeliveryAttempts                              = 8
	DeliveryLease                                 = 2 * time.Minute
)

// DeliveryFailure is safe to expose; provider adapters must not copy raw errors.
type DeliveryFailure struct {
	Message    string
	RetryAfter time.Duration
	Global     bool
	Permanent  bool
	Uncertain  bool
	Blocked    bool
}

func (e *DeliveryFailure) Error() string { return e.Message }

// DeliveryState hides worker phases while preserving actionable uncertainty.
func DeliveryState(state model.DeliveryState) string {
	switch state {
	case model.DeliveryDelivered:
		return "delivered"
	case model.DeliveryFailed, MessageCredentialsBlocked, MessageDestinationBlocked:
		return "failed"
	case MessageUncertain, MessageGlobalUncertain, MessageReconciling:
		return "uncertain"
	default:
		return "pending"
	}
}

// FailedMessage includes operational pauses that need explicit intervention.
func FailedMessage(state model.DeliveryState) bool {
	return state == model.DeliveryFailed || state == MessageCredentialsBlocked || state == MessageDestinationBlocked
}

// FailureStates returns the persisted states represented as Failed in the UI.
func FailureStates() []model.DeliveryState {
	return []model.DeliveryState{model.DeliveryFailed, MessageCredentialsBlocked, MessageDestinationBlocked}
}
