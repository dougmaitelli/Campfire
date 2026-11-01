package service

import (
	"strings"
	"testing"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/utils"
)

func TestOIDCPreferredUsername(t *testing.T) {
	for _, test := range []struct {
		name, claim, want string
	}{
		{name: "available", claim: "player.one", want: "player.one"},
		{name: "normalized", claim: " Player.One! ", want: "player.one"},
		{name: "missing", want: "oidc"},
		{name: "invalid characters", claim: "@@@", want: "oidc"},
		{name: "short", claim: "ab", want: "ab-oidc"},
		{name: "maximum length", claim: strings.Repeat("a", limits.UsernameMax), want: strings.Repeat("a", limits.UsernameMax)},
		{name: "too long", claim: strings.Repeat("a", limits.UsernameMax+1), want: strings.Repeat("a", limits.UsernameMax)},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth, _, p, config := oidcFixture(t)

			provider, err := NewOIDCService(t.Context(), auth, config)
			if err != nil {
				t.Fatal(err)
			}

			cookie, state := p.flow(t, provider, 0, "player-subject", map[string]any{"preferred_username": test.claim})

			session, _, err := provider.Complete(t.Context(), cookie, state, "valid")
			if err != nil {
				t.Fatal(err)
			}

			if session.User.Username != test.want || !utils.IsValidUsername(session.User.Username) {
				t.Fatalf("got username %q, want %q", session.User.Username, test.want)
			}

			cookie, state = p.flow(t, provider, 0, "player-subject", map[string]any{"preferred_username": "changed"})

			repeated, _, err := provider.Complete(t.Context(), cookie, state, "valid")
			if err != nil || repeated.UserID != session.UserID || repeated.User.Username != session.User.Username {
				t.Fatal("repeat login changed the account or username", repeated.User, err)
			}
		})
	}
}
