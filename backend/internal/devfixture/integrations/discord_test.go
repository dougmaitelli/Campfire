//go:build !production

package integrations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fixtureRequest(handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), method, "http://localhost"+path, strings.NewReader(body))
	for name, value := range headers {
		r.Header.Set(name, value)
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	return w
}

func TestDiscordViewerSharesMessageEditsAndDeletionWithProvider(t *testing.T) {
	d := NewDiscord()
	api, viewer := d.Handler(), d.ViewerHandler()
	bot := map[string]string{"Authorization": "Bot " + DiscordToken}
	controls := map[string]string{"X-Campfire-Mock": "discord"}

	create := fixtureRequest(api, "POST", "/channels/400/messages", `{"nonce":"one","content":"<script>unsafe</script>","embeds":[{"title":"Server","fields":[{"name":"Players","value":"2"}]}]}`, bot)
	if create.Code != http.StatusOK {
		t.Fatal(create.Code, create.Body.String())
	}

	if again := fixtureRequest(api, "POST", "/channels/400/messages", `{"nonce":"one"}`, bot); again.Body.String() != create.Body.String() {
		t.Fatal("nonce did not reuse the accepted message", again.Body.String())
	}

	patch := fixtureRequest(api, "PATCH", "/channels/400/messages/500", `{"embeds":[{"title":"Updated server"}]}`, bot)
	if patch.Code != http.StatusOK {
		t.Fatal(patch.Code)
	}

	state := fixtureRequest(viewer, "GET", "/mock/discord/state", "", nil)

	var snapshot struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(state.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}

	if len(snapshot.Messages) != 1 || snapshot.Messages[0]["id"] != "500" || snapshot.Messages[0]["edited_timestamp"] == nil || snapshot.Messages[0]["content"] != "<script>unsafe</script>" {
		t.Fatal("viewer lost message identity, edit, or content", state.Body.String())
	}

	if removed := fixtureRequest(viewer, "DELETE", "/mock/discord/messages/500", "", controls); removed.Code != http.StatusNoContent {
		t.Fatal(removed.Code)
	}

	missing := fixtureRequest(api, "GET", "/channels/400/messages/500", "", bot)
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "10008") {
		t.Fatal("viewer deletion did not reach the provider", missing.Body.String())
	}

	recreated := fixtureRequest(api, "POST", "/channels/400/messages", `{"nonce":"one"}`, bot)
	if !strings.Contains(recreated.Body.String(), `"id":"501"`) {
		t.Fatal("deleted nonce resurrected an absent receipt", recreated.Body.String())
	}

	d.Reset()

	if state := fixtureRequest(viewer, "GET", "/mock/discord/state", "", nil); !strings.Contains(state.Body.String(), `"messages":[]`) {
		t.Fatal("reset retained provider messages", state.Body.String())
	}
}

func TestDiscordViewerControlsRequireLocalIntentAndIsolateChannels(t *testing.T) {
	d := NewDiscord()
	api, viewer := d.Handler(), d.ViewerHandler()
	bot := map[string]string{"Authorization": "Bot " + DiscordToken}
	controls := map[string]string{"X-Campfire-Mock": "discord"}

	for _, headers := range []map[string]string{
		nil,
		{"X-Campfire-Mock": "discord", "Origin": "https://other.example"},
		{"X-Campfire-Mock": "discord", "Sec-Fetch-Site": "cross-site"},
	} {
		if result := fixtureRequest(viewer, "PUT", "/mock/discord/channels/400", `{"failure":"forbidden"}`, headers); result.Code != http.StatusForbidden {
			t.Fatal("accepted a cross-site or unsigned control", result.Code)
		}
	}

	for _, scenario := range []struct {
		mode   string
		status int
	}{
		{"forbidden", http.StatusForbidden},
		{"rate-limit", http.StatusTooManyRequests},
		{"unavailable", http.StatusServiceUnavailable},
		{"", http.StatusOK},
	} {
		changed := fixtureRequest(viewer, "PUT", "/mock/discord/channels/400", `{"failure":"`+scenario.mode+`"}`, controls)
		if changed.Code != http.StatusNoContent {
			t.Fatal(changed.Code)
		}

		result := fixtureRequest(api, "POST", "/channels/400/messages", `{"content":"hello"}`, bot)
		if result.Code != scenario.status {
			t.Fatal(scenario.mode, result.Code)
		}

		if scenario.mode == "rate-limit" && (result.Header().Get("Retry-After") != "5" || !strings.Contains(result.Body.String(), `"retry_after":5`)) {
			t.Fatal("rate limit lacks retry instructions")
		}

		if unaffected := fixtureRequest(api, "POST", "/channels/402/messages", `{"content":"hello"}`, bot); unaffected.Code != http.StatusOK {
			t.Fatal("one channel failure blocked another", unaffected.Code)
		}
	}

	if bad := fixtureRequest(viewer, "PUT", "/mock/discord/channels/400", `{"failure":"unknown"}`, controls); bad.Code != http.StatusBadRequest {
		t.Fatal("unknown failure accepted")
	}

	if leaked := fixtureRequest(api, "GET", "/mock/discord/state", "", bot); leaked.Code != http.StatusNotFound {
		t.Fatal("inspection routes leaked into provider API")
	}
}
