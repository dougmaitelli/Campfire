package agent

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/shared/protocol"
)

func TestGameRegistrationEndpointNegotiationAndAcknowledgement(t *testing.T) {
	directory, err := os.MkdirTemp("", "game-register-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(directory) }()

	g, err := newGameIntegrations(t.Context(), directory, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()

	path, err := g.mount("game")
	if err != nil {
		t.Fatal(err)
	}

	client := gameClient(filepath.Join(path, "publish.sock"))
	defer client.CloseIdleConnections()

	body := `{"version":1,"registration":{"revision":1,"state_fields":{"map":"World"},"event_fields":{"player.joined":{"player":"Player name"}}}}`
	request := func(method, path, body string, want int) {
		t.Helper()

		r, err := http.NewRequestWithContext(t.Context(), method, "http://spark"+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}

		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}

		detail, _ := io.ReadAll(response.Body)

		_ = response.Body.Close()
		if response.StatusCode != want {
			t.Fatal("registration response", response.StatusCode, string(detail))
		}
	}
	request("PUT", "/v1/registration", body, http.StatusServiceUnavailable)
	g.attach(t.Context(), func(protocol.Message) error { t.Fatal("unnegotiated registration forwarded"); return nil }, true)
	request("PUT", "/v1/registration", body, http.StatusServiceUnavailable)

	received := make(chan protocol.GameFrame, 1)

	g.attach(t.Context(), func(message protocol.Message) error {
		received <- *message.Game
		return g.acknowledge(&protocol.GameAck{ID: message.Game.ID})
	}, true, true)
	request("PUT", "/v1/registration", body, http.StatusNoContent)

	frame := <-received
	if frame.ServerID != "game" || frame.Publication.Registration.StateFields["map"] != "World" {
		t.Fatal("registration identity or metadata lost", frame)
	}

	request("POST", "/v1/publish", body, http.StatusBadRequest)
	request("PUT", "/v1/registration", `{"version":1,"state":{}}`, http.StatusBadRequest)
	g.attach(t.Context(), func(message protocol.Message) error {
		return g.acknowledge(&protocol.GameAck{ID: message.Game.ID, Error: &protocol.CommandError{Code: protocol.ErrorPublicationRejected, Message: "Stale revision"}})
	}, true, true)
	request("PUT", "/v1/registration", body, http.StatusConflict)
}
