package discord

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBotInviteUsesPublicIdentityAndCentralPermissionPolicy(t *testing.T) {
	link, err := url.Parse(BotInviteURL("100", "200"))
	if err != nil {
		t.Fatal(err)
	}

	query := link.Query()
	if link.Scheme != "https" || link.Host != "discord.com" || link.Path != "/oauth2/authorize" || query.Get("client_id") != "100" || query.Get("scope") != "bot" || query.Get("permissions") != "84992" || query.Get("guild_id") != "200" || len(query) != 4 {
		t.Fatal("unexpected bot grant", link)
	}

	if BotInviteURL("not-a-bot-id", "200") != "" {
		t.Fatal("invalid identity accepted")
	}
}

func TestRequiredBotPermissionsIncludeEverySetupRequirement(t *testing.T) {
	if RequiredBotPermissions != 84992 || len(RequiredPermissionNames()) != 4 {
		t.Fatal("required permission mask or guidance is inconsistent", RequiredBotPermissions, RequiredPermissionNames())
	}
}

func TestEffectivePermissionsHonorChannelOverridePrecedence(t *testing.T) {
	for _, test := range []struct {
		name      string
		base      uint64
		owner     bool
		overrides []overwrite
		available bool
	}{
		{"base role", RequiredBotPermissions, false, nil, true},
		{"missing history", RequiredBotPermissions &^ readHistory, false, nil, false},
		{"everyone denies", RequiredBotPermissions, false, []overwrite{{ID: "200", Type: 0, Allow: "0", Deny: "2048"}}, false},
		{"role restores", RequiredBotPermissions, false, []overwrite{{ID: "200", Type: 0, Allow: "0", Deny: "2048"}, {ID: "300", Type: 0, Allow: "2048", Deny: "0"}}, true},
		{"member denies", RequiredBotPermissions, false, []overwrite{{ID: "100", Type: 1, Allow: "0", Deny: "2048"}}, false},
		{"member restores", RequiredBotPermissions, false, []overwrite{{ID: "300", Type: 0, Allow: "0", Deny: "2048"}, {ID: "100", Type: 1, Allow: "2048", Deny: "0"}}, true},
		{"administrator bypass", administrator, false, []overwrite{{ID: "100", Type: 1, Allow: "0", Deny: "84992"}}, true},
		{"owner bypass", 0, true, nil, true},
		{"other member ignored", RequiredBotPermissions, false, []overwrite{{ID: "999", Type: 1, Allow: "0", Deny: "84992"}}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			bits, err := effectivePermissions(test.base, "200", "100", []string{"300"}, test.overrides, test.owner)
			if err != nil || (bits&RequiredBotPermissions == RequiredBotPermissions) != test.available {
				t.Fatal("effective channel permissions are wrong", bits, err)
			}
		})
	}

	if bits, err := permissions("18446744073709636608"); err != nil || bits&RequiredBotPermissions != RequiredBotPermissions {
		t.Fatal("higher Discord permission bits broke the required mask", bits, err)
	}
}

func TestClientRejectsRedirectsAndDoesNotExposeProviderResponses(t *testing.T) {
	var received atomic.Bool

	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { received.Store(true) }))
	defer target.Close()

	for _, status := range []int{401, 403, 404, 429, 500, 302} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", target.URL)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"private token: secret"}`))
		}))
		_, err := NewClient(server.URL, server.Client()).Preview(context.Background(), "private-secret")
		server.Close()

		if err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "token: secret") {
			t.Fatal("provider failure was accepted or exposed response contents", status, err)
		}
	}

	if received.Load() {
		t.Fatal("credential-bearing redirect was followed")
	}
}
