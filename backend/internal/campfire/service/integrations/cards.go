package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

// CardSource supplies current public data independently of any provider.
type CardSource interface {
	IntegrationCard(context.Context, string, core.CardSettings) (core.ServerCard, error)
}

// CardProvider owns provider access and external singleton-message operations.
// Check enforces publishing permissions; reads and deletion use their native
// permissions so losing Send Messages does not prevent receipt recovery/cleanup.
type CardProvider interface {
	Fingerprint(core.ServerCard) string
	Check(context.Context, repository.IntegrationClaim) error
	CreateCard(context.Context, repository.CardClaim, core.ServerCard) (string, error)
	FindCard(context.Context, repository.CardClaim) (string, error)
	CardExists(context.Context, repository.CardClaim) (bool, error)
	EditCard(context.Context, repository.CardClaim, core.ServerCard) (bool, error)
	DeleteCard(context.Context, repository.CardClaim) error
}

// CardWorker maintains singleton messages, separately from event delivery.
type CardWorker struct {
	Store     *repository.IntegrationRepository
	Source    CardSource
	Providers map[string]CardProvider
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// Start runs independent destinations with a bounded number of active workers.
func (w *CardWorker) Start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)
	w.wg.Go(func() {
		slots := make(chan struct{}, 4)

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}

			claim, err := w.Store.ClaimCard(ctx, time.Now())
			if err != nil || claim == nil {
				<-slots

				if err != nil && ctx.Err() == nil {
					slog.Error("reconcile integration card", "error", err)
				}

				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}

				continue
			}

			w.wg.Go(func() {
				defer func() { <-slots }()

				if err := w.reconcile(ctx, *claim); err != nil && ctx.Err() == nil {
					slog.Error("reconcile integration card", "error", err)
				}
			})
		}
	})
}

// Close cancels HTTP and waits for durable outcome persistence.
func (w *CardWorker) Close() {
	if w.cancel != nil {
		w.cancel()
		w.wg.Wait()
	}
}

// RunOnce processes at most one due binding without starting background workers.
func (w *CardWorker) RunOnce(ctx context.Context) error {
	claim, err := w.Store.ClaimCard(ctx, time.Now())
	if err != nil || claim == nil {
		return err
	}

	return w.reconcile(ctx, *claim)
}

func cardHash(content any) string {
	payload, _ := json.Marshal(content)
	digest := sha256.Sum256(payload)

	return hex.EncodeToString(digest[:])
}

func cardFailure(err error) *core.DeliveryFailure {
	var failure *core.DeliveryFailure
	if errors.As(err, &failure) {
		return failure
	}

	return &core.DeliveryFailure{Message: "The server card could not be updated. Check the integration connection."}
}

// reconcile coalesces updates to five seconds and checks quiet messages every
// minute. An uncertain creation can only find/adopt its existing receipt.
func (w *CardWorker) reconcile(ctx context.Context, claim repository.CardClaim) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	now := time.Now()
	work, ref := claim.Work, claim.Binding.StatusMessage
	active := claim.Binding.Enabled && claim.Connection.Enabled && claim.Binding.Card.Enabled
	state, message, next := core.CardReady, "", now.Add(5*time.Second)
	provider := w.Providers[claim.Connection.Provider]
	creating := false

	err := func() error {
		if provider == nil {
			return &core.DeliveryFailure{Message: "This integration provider is not available.", Permanent: true}
		}

		if work.Creating {
			id, err := provider.FindCard(ctx, claim)
			if err != nil {
				return err
			}

			if id == "" {
				return &core.DeliveryFailure{Message: "The provider may have accepted this server card, but its receipt could not be confirmed. Automatic creation is paused to avoid duplicates. Check the card to search again.", Permanent: true, Uncertain: true}
			}

			ref = core.MessageReference{DestinationID: claim.DestinationID, ID: id}
			work.Creating, work.CheckedAt, work.Hash = false, now.Unix(), ""

			return nil
		}

		if ref.ID != "" && (!active || ref.DestinationID != claim.Binding.DestinationID) {
			if err := w.Store.CheckCardMutation(ctx, claim, true); err != nil {
				return err
			}

			if err := provider.DeleteCard(ctx, claim); err != nil {
				return err
			}

			ref, work = core.MessageReference{}, core.CardWork{Version: 1}

			if !active {
				next = time.Time{}
			}

			return nil
		}

		if !active {
			next = time.Time{}
			return nil
		}

		card, err := w.Source.IntegrationCard(ctx, claim.Binding.ServerID, claim.Binding.Card)
		if err != nil {
			return err
		}

		fingerprint := provider.Fingerprint(card)
		if ref.ID != "" && work.Hash == fingerprint && now.Unix()-work.CheckedAt < 60 {
			return nil
		}

		if err := provider.Check(ctx, claim.IntegrationClaim); err != nil {
			return err
		}

		// Permission checks can take time. Rebuild from current observations before
		// a mutation rather than publishing a snapshot that expired during HTTP.
		card, err = w.Source.IntegrationCard(ctx, claim.Binding.ServerID, claim.Binding.Card)
		if err != nil {
			return err
		}

		fingerprint = provider.Fingerprint(card)

		if ref.ID == "" {
			if err := w.Store.BeginCardCreate(ctx, &claim, now); err != nil {
				return err
			}

			work, creating = claim.Work, true

			id, err := provider.CreateCard(ctx, claim, card)
			if err != nil {
				return err
			}

			if id == "" {
				return &core.DeliveryFailure{Message: "The provider returned no server-card receipt. Delivery will be checked before creating another card.", Uncertain: true}
			}

			ref = core.MessageReference{DestinationID: claim.DestinationID, ID: id}
			work.Creating, work.Hash, work.CheckedAt = false, fingerprint, now.Unix()

			return nil
		}

		if err := w.Store.CheckCardMutation(ctx, claim, false); err != nil {
			return err
		}

		var found bool
		if work.Hash != fingerprint {
			found, err = provider.EditCard(ctx, claim, card)
		} else {
			found, err = provider.CardExists(ctx, claim)
		}

		if err != nil {
			return err
		}

		if !found {
			ref, work = core.MessageReference{}, core.CardWork{Version: 1}
			return nil
		}

		work.Hash, work.CheckedAt = fingerprint, now.Unix()

		return nil
	}()
	if err != nil {
		failure := cardFailure(err)

		message = failure.Message
		if creating && !failure.Uncertain {
			work.Creating = false
		}

		delay := max(5*time.Second*time.Duration(1<<min(claim.Attempts-1, 9)), failure.RetryAfter)
		next = time.Now().Add(delay).Truncate(time.Second).Add(time.Second)

		if failure.RetryAfter > 0 {
			state = core.MessageRateLimited
			if failure.Global {
				state = core.MessageGlobalLimited
				if work.Creating {
					state = core.MessageGlobalUncertain
				}
			}
		}

		if failure.Blocked {
			state = core.MessageDestinationBlocked
			if failure.Global {
				state = core.MessageCredentialsBlocked
			}
		}

		if failure.Permanent || (claim.Attempts >= core.DeliveryAttempts && failure.RetryAfter == 0) {
			next = time.Time{}

			if !core.FailedMessage(state) {
				state = model.DeliveryFailed
			}
		}
	}

	finish, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()

	return w.Store.FinishCard(finish, claim, work, ref, state, message, next)
}
