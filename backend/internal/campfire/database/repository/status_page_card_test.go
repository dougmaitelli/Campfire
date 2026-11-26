package repository_test

import (
	"errors"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/statuspage"
)

func TestValidatedPageChannelResumesPublishing(t *testing.T) {
	db, store, _ := integrationStore(t)
	ctx := t.Context()
	now := time.Now()

	claim, err := store.ClaimCard(ctx, now)
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}

	if err := store.FinishCard(ctx, *claim, claim.Work, claim.Binding.StatusMessage, core.MessageDestinationBlocked, "permission denied", time.Time{}); err != nil {
		t.Fatal(err)
	}

	pages := repository.NewStatusPageRepository(db.ORM())

	page, err := pages.SaveWithDiscord(ctx, "", statuspage.Input{Title: "Public games", Slug: "games", DiscordChannelID: &claim.DestinationID}, claim.Connection)
	if err != nil {
		t.Fatal(err)
	}

	claim, err = store.ClaimCard(ctx, now.Add(time.Minute))
	if err != nil || claim == nil || claim.Binding.StatusPageID != page.ID {
		t.Fatal("validated destination stayed blocked", claim, err)
	}
}

func TestDeletedPageReclaimsExpiredLease(t *testing.T) {
	t.Run("before send", func(t *testing.T) { testDeletedPageLease(t, false) })
	t.Run("unconfirmed send", func(t *testing.T) { testDeletedPageLease(t, true) })
}

func testDeletedPageLease(t *testing.T, sending bool) {
	t.Helper()
	db, store, _ := integrationStore(t)

	ctx := t.Context()
	if err := store.DeleteBinding(ctx, "binding"); err != nil {
		t.Fatal(err)
	}

	conn, err := store.GetConnection(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}

	pages := repository.NewStatusPageRepository(db.ORM())

	page, err := pages.SaveWithDiscord(ctx, "", statuspage.Input{Title: "Games", Slug: "games", DiscordChannelID: new("channel")}, conn)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()

	claim, err := store.ClaimCard(ctx, now)
	if err != nil || claim == nil || claim.Binding.StatusPageID != page.ID || claim.Work.Creating {
		t.Fatal(claim, err)
	}

	if sending {
		if err := store.BeginCardCreate(ctx, claim, now); err != nil {
			t.Fatal(err)
		}
	}

	if err := pages.Delete(ctx, page.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := store.PageBinding(ctx, page.ID); err != nil {
		t.Fatal("lost a live lease", err)
	}

	recovered, err := store.ClaimCard(ctx, now.Add(core.DeliveryLease+time.Second))
	if err != nil {
		t.Fatal(err)
	}

	if sending {
		if recovered == nil || !recovered.Work.Creating || recovered.Work.CreationID != claim.Work.CreationID {
			t.Fatal("lost an unconfirmed send after its lease expired", recovered)
		}

		return
	}

	if _, err := store.PageBinding(ctx, page.ID); !errors.Is(err, repository.ErrIntegrationNotFound) {
		t.Fatal("abandoned pre-send lease survived", err)
	}

	if err := store.BeginCardCreate(ctx, claim, now); err == nil {
		t.Fatal("stale worker could still send")
	}
}

func TestFailedDeletedPageCleanupRemainsRetryableAndReleasesChannel(t *testing.T) {
	db, store, _ := integrationStore(t)
	ctx := t.Context()

	server, err := store.GetBinding(ctx, "binding")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteBinding(ctx, server.ID); err != nil {
		t.Fatal(err)
	}

	conn, err := store.GetConnection(ctx, server.IntegrationID)
	if err != nil {
		t.Fatal(err)
	}

	pages := repository.NewStatusPageRepository(db.ORM())

	page, err := pages.SaveWithDiscord(ctx, "", statuspage.Input{Title: "Community games", Slug: "games", DiscordChannelID: &server.DestinationID}, conn)
	if err != nil {
		t.Fatal(err)
	}

	binding, err := store.PageBinding(ctx, page.ID)
	if err != nil {
		t.Fatal(err)
	}

	ref := core.MessageReference{DestinationID: server.DestinationID, ID: "published"}
	if err := seedIntegrationCardReference(ctx, db, binding.ID, ref); err != nil {
		t.Fatal(err)
	}

	if err := pages.Delete(ctx, page.ID); err != nil {
		t.Fatal(err)
	}

	if _, _, err := pages.Published(ctx, page.Slug); !errors.Is(err, statuspage.ErrNotFound) {
		t.Fatal("page still public", err)
	}

	now := time.Now()

	claim, err := store.ClaimCard(ctx, now)
	if err != nil || claim == nil {
		t.Fatal(claim, err)
	}

	if err := store.FinishCard(ctx, *claim, claim.Work, ref, core.MessageDestinationBlocked, "permission denied", time.Time{}); err != nil {
		t.Fatal(err)
	}

	rows, err := pages.DiscordCardCleanups(ctx)
	if err != nil || len(rows) != 1 || rows[0].ID != page.ID || rows[0].Title != page.Title || rows[0].ChannelID != ref.DestinationID || !rows[0].Status.CanRetry {
		t.Fatal(rows, err)
	}

	if status, err := pages.DiscordCardStatus(ctx, page.ID); err != nil || status != rows[0].Status {
		t.Fatal(status, err)
	}

	if err := store.CreateBinding(ctx, server); err != nil {
		t.Fatal(err)
	}

	other, err := store.ClaimCard(ctx, now)
	if err != nil || other == nil || other.BindingID != server.ID {
		t.Fatal("cleanup blocked another card", other, err)
	}

	if err := store.FinishCard(ctx, *other, other.Work, core.MessageReference{}, core.CardReady, "", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if err := pages.RetryDiscordCard(ctx, page.ID); err != nil {
		t.Fatal(err)
	}

	claim, err = store.ClaimCard(ctx, now.Add(time.Minute))
	if err != nil || claim == nil || claim.Binding.StatusMessage != ref {
		t.Fatal("lost Discord receipt on retry", claim, err)
	}

	if err := store.FinishCard(ctx, *claim, claim.Work, core.MessageReference{}, core.CardReady, "", time.Time{}); err != nil {
		t.Fatal(err)
	}

	if rows, err := pages.DiscordCardCleanups(ctx); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}

	if _, err := pages.DiscordCardStatus(ctx, page.ID); !errors.Is(err, statuspage.ErrNotFound) {
		t.Fatal(err)
	}
}
