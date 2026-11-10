package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

// MessageProvider owns authorization, rendering, sending, and receipt recovery.
// Other providers can use the same durable queue without Discord dependencies.
type MessageProvider interface {
	Check(context.Context, repository.IntegrationClaim) error
	Send(context.Context, repository.IntegrationClaim) (string, error)
	Find(context.Context, repository.IntegrationClaim) (string, error)
}

// DeliveryWorker runs independent slots over a shared durable queue.
type DeliveryWorker struct {
	Store     *repository.IntegrationRepository
	Providers map[string]MessageProvider
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// Start runs bounded slots; provider registration is immutable after startup.
func (w *DeliveryWorker) Start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()

		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		for {
			if err := w.Store.PruneMessageHistory(ctx, time.Now()); err != nil && ctx.Err() == nil {
				slog.Error("prune integration notification history", "error", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	for range 4 {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()

			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				claim, err := w.Store.ClaimMessage(ctx, time.Now())
				if err == nil && claim != nil {
					err = w.deliver(ctx, *claim)
				}

				if err != nil && ctx.Err() == nil {
					slog.Error("deliver integration notifications", "error", err)
				}

				if err == nil && claim != nil {
					continue
				}

				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
}

// Close cancels HTTP work and waits for outcome persistence before database close.
func (w *DeliveryWorker) Close() {
	if w.cancel != nil {
		w.cancel()
		w.wg.Wait()
	}
}

// RunOnce claims at most four distinct destinations; a slow channel does not
// serialize unrelated channels. Provider calls never hold database write locks.
func (w *DeliveryWorker) RunOnce(ctx context.Context) error {
	var claims []*repository.IntegrationClaim

	for range 4 {
		claim, err := w.Store.ClaimMessage(ctx, time.Now())
		if err != nil {
			return err
		}

		if claim == nil {
			break
		}

		claims = append(claims, claim)
	}

	results := make(chan error, len(claims))

	var workers sync.WaitGroup
	for _, claim := range claims {
		workers.Go(func() { results <- w.deliver(ctx, *claim) })
	}

	workers.Wait()
	close(results)

	var result error
	for err := range results {
		result = errors.Join(result, err)
	}

	return result
}

func deliveryFailure(err error) *core.DeliveryFailure {
	var failure *core.DeliveryFailure
	if errors.As(err, &failure) {
		return failure
	}

	return &core.DeliveryFailure{Message: "The notification could not be delivered. Check the integration connection."}
}

func (w *DeliveryWorker) deliver(ctx context.Context, claim repository.IntegrationClaim) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	provider := w.Providers[claim.Connection.Provider]

	var externalID string

	var err error
	if provider == nil {
		err = &core.DeliveryFailure{Message: "This integration provider is not available.", Permanent: true}
	} else if err = provider.Check(ctx, claim); err == nil {
		if claim.Reconcile {
			externalID, err = provider.Find(ctx, claim)
			if err == nil && externalID == "" {
				err = &core.DeliveryFailure{Message: "The provider may have accepted this notification, but delivery could not be confirmed. Automatic resending is paused to avoid duplicates.", Permanent: true, Uncertain: true}
			}
		} else if err = w.Store.BeginMessageSend(ctx, claim); err == nil {
			externalID, err = provider.Send(ctx, claim)
		}
	}

	if err == nil && externalID == "" {
		err = &core.DeliveryFailure{Message: "The provider returned no delivery receipt. Delivery will be checked before another message is sent.", Uncertain: true}
	}

	now := time.Now()
	state, message, next := model.DeliveryDelivered, "", time.Time{}

	if err != nil {
		externalID = ""
		failure := deliveryFailure(err)
		message, state = failure.Message, model.DeliveryPending
		delay := max(5*time.Second*time.Duration(1<<min(claim.Attempts-1, 9)), failure.RetryAfter)
		next = now.Add(delay).Truncate(time.Second).Add(time.Second)

		if failure.RetryAfter > 0 {
			state = core.MessageRateLimited
			if failure.Global {
				state = core.MessageGlobalLimited
			}
		}

		if claim.Reconcile || failure.Uncertain {
			state = core.MessageUncertain
			if failure.Global {
				state = core.MessageGlobalUncertain
			}
		}

		if failure.Blocked && !claim.Reconcile && !failure.Uncertain {
			state = core.MessageDestinationBlocked
			if failure.Global {
				state = core.MessageCredentialsBlocked
			}
		}

		if failure.Permanent || (claim.Attempts >= core.DeliveryAttempts && failure.RetryAfter == 0) {
			next = time.Time{}

			if state != core.MessageUncertain && state != core.MessageGlobalUncertain && !core.FailedMessage(state) {
				state = model.DeliveryFailed
			}
		}
	}

	finish, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()

	return w.Store.FinishMessage(finish, claim, state, externalID, message, next, now)
}

func readDeliveryEvent(claim repository.IntegrationClaim) (core.Event, error) {
	var event core.Event
	if json.Unmarshal(claim.Payload, &event) != nil || event.Validate() != nil {
		return event, &core.DeliveryFailure{Message: "The saved notification payload is invalid.", Permanent: true}
	}

	return event, nil
}
