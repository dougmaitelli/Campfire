package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestConsoleStreamingAndLiveRevocation(t *testing.T) {
	t.Run("session", func(t *testing.T) { testConsoleStreamingAndLiveRevocation(t, "session") })
	t.Run("api-token", func(t *testing.T) { testConsoleStreamingAndLiveRevocation(t, "api-token") })
	t.Run("view-as", func(t *testing.T) { testConsoleStreamingAndLiveRevocation(t, "view-as") })
}

func testConsoleStreamingAndLiveRevocation(t *testing.T, mode string) {
	f := newAccessFixture(t)
	user, member := f.user("console-user", database.UserRole)
	_, _, input := serverFixture(t, f.admin)

	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))
	if response.Code != http.StatusCreated {
		t.Fatal(response.Code, response.Body)
	}

	var server model.Server
	if err := json.Unmarshal(response.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}

	endpoint := "/api/v1/servers/" + server.ID
	expectAccess(t, f.admin, "POST", endpoint+"/members", `{"username":"console-user","role_id":5}`, http.StatusOK)

	revoke := func() {
		expectAccess(t, f.admin, "DELETE", endpoint+"/members/"+fmt.Sprint(user.ID), "", http.StatusNoContent)
	}

	if mode == "view-as" {
		_, member = previewAs(t, f.admin, user.ID)
	}

	if mode == "api-token" {
		token, secret := createTestAPIToken(t, member, `["server.view","server.operate"]`)
		browser := member
		member = bearerAPI(f.panel, secret)
		revoke = func() {
			expectAccess(t, browser, "DELETE", fmt.Sprintf("/api/v1/tokens/%d", token.ID), "", http.StatusNoContent)
		}
	}

	cancelled := make(chan struct{})
	f.runtime.console = func(ctx context.Context, id string, emit func(model.ConsoleEvent) error) error {
		defer close(cancelled)

		if id != server.ID {
			return fmt.Errorf("wrong server %s", id)
		}

		if err := emit(model.ConsoleEvent{Ready: true}); err != nil {
			return err
		}

		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				if err := emit(model.ConsoleEvent{Content: "live output\nnext line <script>"}); err != nil {
					return err
				}
			}
		}
	}

	host := httptest.NewServer(member)
	defer host.Close()

	client := host.Client()
	client.Timeout = 5 * time.Second

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, host.URL+endpoint+"/console/stream", nil)
	if err != nil {
		t.Fatal(err)
	}

	stream, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = stream.Body.Close() }()

	if stream.StatusCode != http.StatusOK || stream.Header.Get("Content-Type") != "text/event-stream" || stream.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatal("not an unbuffered event stream", stream.StatusCode, stream.Header)
	}

	scanner := bufio.NewScanner(stream.Body)
	output, denied := false, false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `live output\nnext line`) {
			output = true

			revoke()
		}

		if line == "event: denied" {
			denied = true
			break
		}

		if output {
			break
		}
	}
	// Continue after revocation until the stream explicitly closes access.
	for !denied && scanner.Scan() {
		if scanner.Text() == "event: denied" {
			denied = true
		}
	}

	if !output || !denied {
		t.Fatal("output or revocation notification missing", scanner.Err())
	}

	_ = stream.Body.Close()

	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("closed browser leaked console subscription")
	}
}
