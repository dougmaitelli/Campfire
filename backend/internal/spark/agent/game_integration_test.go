package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/agents"
	"campfire/internal/campfire/devfixture"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func gameClient(path string) *http.Client {
	return &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}}
}

func TestDashboardConfigurationControlsGameEndpoint(t *testing.T) {
	store := &ConfigurationStore{Directory: t.TempDir()}
	d := deploymentFixture("node", "game")

	directory, err := os.MkdirTemp("", "game-")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.RemoveAll(directory) }()

	g, err := newGameIntegrations(t.Context(), directory, func(id string) bool { return store.savedGame("node", id) })
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()

	store.games = g

	for _, enabled := range []bool{false, true, false, true} {
		d.Configuration.GameIntegration = enabled
		if _, err := store.Deploy(t.Context(), "node", d); err != nil {
			t.Fatal(err)
		}

		path, err := g.mount(d.ServerID)
		if !enabled {
			if err == nil || store.savedGame("node", d.ServerID) {
				t.Fatal("disabled integration authorized")
			}

			g.mu.Lock()
			endpoint := g.endpoints[d.ServerID]
			g.mu.Unlock()

			if endpoint != nil {
				t.Fatal("disabled integration retained its listener")
			}

			continue
		}

		if err != nil {
			t.Fatal(err)
		}

		client := gameClient(filepath.Join(path, "publish.sock"))

		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://spark/v1/events/queue", nil)
		if err != nil {
			t.Fatal(err)
		}

		response, err := client.Do(request)
		client.CloseIdleConnections()

		if err != nil {
			t.Fatal(err)
		}

		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatal("enabled endpoint unavailable", response.StatusCode)
		}
	}
}

func testWireGameIntegration(t *testing.T, hub *agents.Hub, sparkID, directory string, received <-chan protocol.GameFrame) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	d := deploymentFixture(sparkID, "game-bridge")

	d.Configuration.GameIntegration = true
	if _, err := hub.Deploy(ctx, sparkID, d); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"install", "start", "stop"} {
		status, err := hub.Action(ctx, sparkID, model.RuntimeRequest{ServerID: d.ServerID, Action: model.ServerAction(action), OperationID: "game-" + action})
		if err != nil {
			t.Fatal(err)
		}

		for status.Busy {
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}

			time.Sleep(5 * time.Millisecond)

			status, err = hub.Action(ctx, sparkID, model.RuntimeRequest{ServerID: d.ServerID, Action: "status"})
			if err != nil {
				t.Fatal(err)
			}
		}

		if status.Error != "" {
			t.Fatal(status.Error)
		}
	}

	hash := sha256.Sum256([]byte(d.ServerID))

	client := gameClient(filepath.Join(directory, "game-integrations", hex.EncodeToString(hash[:16]), "publish.sock"))
	defer client.CloseIdleConnections()

	for _, body := range []string{
		`{"version":1,"state":{"players":3,"capacity":20,"fields":{"map":"world"}}}`,
		`{"version":1,"event":{"id":"joined-1","type":"player.joined","occurred_at":"2026-10-24T00:00:00Z","title":"Player joined"}}`,
	} {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://spark/v1/publish", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}

		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}

		data, _ := io.ReadAll(response.Body)

		_ = response.Body.Close()

		expected := http.StatusNoContent
		if strings.Contains(body, `"event"`) {
			expected = http.StatusAccepted
		}

		if response.StatusCode != expected {
			t.Fatalf("publish: %d %s", response.StatusCode, data)
		}

		select {
		case frame := <-received:
			if frame.ServerID != d.ServerID || frame.Publication.Validate() != nil {
				t.Fatal("invalid received identity or publication", frame)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}

	data, err := devfixture.GameScreenshot()
	if err != nil {
		t.Fatal(err)
	}

	for range 2 {
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://spark/v1/events/joined-1/media/shot", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}

		request.Header.Set("Content-Type", "image/png")
		request.Header.Set("X-Campfire-Media-Name", "world.png")

		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}

		body, _ := io.ReadAll(response.Body)

		_ = response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("media upload/retry failed: %d %s", response.StatusCode, body)
		}
	}
}

func TestGamePublishingBoundsAndDisconnect(t *testing.T) {
	g := &gameIntegrations{authorize: func(id string) bool { return id == "one" }, slots: make(chan struct{}, 32)}

	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"version":1,"state":{}}`, http.StatusServiceUnavailable},
		{`{"version":1,"state":{},"server_id":"other"}`, http.StatusBadRequest},
		{`{"version":2,"state":{}}`, http.StatusBadRequest},
		{`{"version":1,"state":{}} {}`, http.StatusBadRequest},
		{`{"version":1,"state":{"fields":{"map":{}}}}`, http.StatusBadRequest},
		{`{"version":1,"state":{"fields":{"map":"` + strings.Repeat("x", model.GamePublicationLimit) + `"}}}`, http.StatusRequestEntityTooLarge},
	} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/publish", strings.NewReader(test.body))
		w := httptest.NewRecorder()
		g.publish(w, r, "one")

		if w.Code != test.status {
			t.Fatalf("got %d, want %d: %s", w.Code, test.status, w.Body.String())
		}
	}

	w := httptest.NewRecorder()
	g.publish(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/publish", strings.NewReader(`{"version":1,"state":{}}`)), "other")

	if w.Code != http.StatusForbidden {
		t.Fatal("unauthorized server accepted")
	}

	store := &ConfigurationStore{}
	if slices.Contains(store.capabilities(), protocol.CapabilityGameIntegration) {
		t.Fatal("disabled integration advertised")
	}

	store.games = g
	if !slices.Contains(store.capabilities(), protocol.CapabilityGameIntegration) {
		t.Fatal("enabled integration not advertised")
	}
}

func TestGameSocketIsolationAndRateLimit(t *testing.T) {
	directory, err := os.MkdirTemp("", "cf-sock-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(directory) }()

	g, err := newGameIntegrations(t.Context(), directory, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()

	one, err := g.mount("one")
	if err != nil {
		t.Fatal(err)
	}

	two, err := g.mount("two")
	if err != nil || one == two {
		t.Fatal("endpoints not isolated", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var source string

	g.attach(ctx, func(message protocol.Message) error {
		source = message.Game.ServerID
		return g.acknowledge(&protocol.GameAck{ID: message.Game.ID})
	})

	client := gameClient(filepath.Join(one, "publish.sock"))
	defer client.CloseIdleConnections()

	limited := false

	for range 11 {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://spark/v1/publish", strings.NewReader(`{"version":1,"state":{}}`))
		if err != nil {
			t.Fatal(err)
		}

		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}

		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		limited = limited || response.StatusCode == http.StatusTooManyRequests
	}

	if source != "one" || !limited {
		t.Fatal("socket identity or rate limit failed", source, limited)
	}

	g.close()

	if _, err := g.mount("one"); err == nil {
		t.Fatal("endpoint reopened after shutdown")
	}

	restarted, err := newGameIntegrations(t.Context(), directory, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.close()

	restored, err := restarted.mount("one")
	if err != nil || restored != one {
		t.Fatal("socket directory changed after restart", restored, err)
	}
}
