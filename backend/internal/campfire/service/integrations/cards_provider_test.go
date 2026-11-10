package integrations

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	fixture "campfire/internal/campfire/devfixture/integrations"
	core "campfire/internal/campfire/integrations"
)

type secondCardProvider struct{ entered, created chan struct{} }

func (*secondCardProvider) Fingerprint(card core.ServerCard) string {
	card.ExpiresAt = 0
	return cardHash(card)
}

func (p *secondCardProvider) Check(ctx context.Context, claim repository.IntegrationClaim) error {
	if claim.DestinationID == "slow" {
		p.entered <- struct{}{}

		<-ctx.Done()

		return ctx.Err()
	}

	return nil
}
func (p *secondCardProvider) CreateCard(context.Context, repository.CardClaim, core.ServerCard) (string, error) {
	p.created <- struct{}{}
	return "provider-native-id", nil
}
func (*secondCardProvider) FindCard(context.Context, repository.CardClaim) (string, error) {
	return "provider-native-id", nil
}
func (*secondCardProvider) CardExists(context.Context, repository.CardClaim) (bool, error) {
	return true, nil
}
func (*secondCardProvider) EditCard(context.Context, repository.CardClaim, core.ServerCard) (bool, error) {
	return true, nil
}
func (*secondCardProvider) DeleteCard(context.Context, repository.CardClaim) error { return nil }

func TestCardWorkerSupportsOtherProvidersAndIndependentDestinations(t *testing.T) {
	_, setup, store, id, _ := cardService(t, fixture.DiscordHandler())
	binding := cardBinding(t, store, id)

	binding.Card.Enabled = false
	if err := store.UpdateBinding(t.Context(), binding); err != nil {
		t.Fatal(err)
	}

	if err := store.CreateConnection(t.Context(), core.Connection{ID: "second-provider", Provider: "test-provider", Name: "Workspace", Enabled: true, Config: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}

	for _, item := range []struct{ id, server, destination string }{{"slow-binding", "first", "slow"}, {"fast-binding", "second", "fast"}} {
		if err := store.CreateBinding(t.Context(), core.Binding{ID: item.id, IntegrationID: "second-provider", ServerID: item.server, DestinationID: item.destination, Enabled: true, Card: core.CardSettings{Enabled: true}}); err != nil {
			t.Fatal(err)
		}
	}

	provider := &secondCardProvider{entered: make(chan struct{}, 1), created: make(chan struct{}, 1)}
	setup.cards.Providers["test-provider"] = provider

	setup.cards.Start(t.Context())
	defer setup.cards.Close()

	for _, signal := range []chan struct{}{provider.entered, provider.created} {
		select {
		case <-signal:
		case <-time.After(3 * time.Second):
			t.Fatal("one destination prevented another provider's card from progressing")
		}
	}
}
