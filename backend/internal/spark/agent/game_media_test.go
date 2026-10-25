package agent

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"campfire/internal/shared/model"
)

func TestLocalGameMediaPlacementAvailabilityAndBackpressure(t *testing.T) {
	g := &gameIntegrations{authorize: func(id string) bool { return id == "game" }, mediaSlots: make(chan struct{}, 2)}

	call := func(server, contentType, name string, size int64) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v1/events/event/media/shot", bytes.NewReader([]byte("image")))
		r.SetPathValue("event", "event")
		r.SetPathValue("attachment", "shot")
		r.Header.Set("Content-Type", contentType)
		r.Header.Set("X-Campfire-Media-Name", name)
		r.ContentLength = size
		w := httptest.NewRecorder()
		g.mediaUpload(w, r, server)

		return w
	}
	if w := call("other", "image/png", "world.png", 5); w.Code != http.StatusForbidden {
		t.Fatal(w.Code)
	}

	if w := call("game", "image/svg+xml", "world.svg", 5); w.Code != http.StatusBadRequest {
		t.Fatal(w.Code)
	}

	if w := call("game", "image/png", "world.png", model.GameMediaFileLimit+1); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatal(w.Code)
	}

	if w := call("game", "image/png", "world.png", 5); w.Code != http.StatusServiceUnavailable {
		t.Fatal(w.Code)
	}

	g.session = &gameSession{ctx: context.Background(), upload: func(_ context.Context, serverID, eventID string, media model.GameMedia, body io.Reader) (*http.Response, error) {
		data, err := io.ReadAll(body)
		if err != nil || string(data) != "image" || serverID != "game" || eventID != "event" || media.ID != "shot" {
			t.Fatal("relay changed source or bytes", err)
		}

		return &http.Response{StatusCode: http.StatusConflict, Header: http.Header{"Retry-After": {"5"}}, Body: io.NopCloser(bytes.NewReader([]byte("event not retained yet")))}, nil
	}}

	if w := call("game", "image/png", "world.png", -1); w.Code != http.StatusConflict || w.Header().Get("Retry-After") != "5" {
		t.Fatal("upstream rejection lost", w.Code)
	}

	for range cap(g.mediaSlots) {
		g.mediaSlots <- struct{}{}
	}

	if w := call("game", "image/png", "world.png", 5); w.Code != http.StatusTooManyRequests {
		t.Fatal("upload concurrency unbounded", w.Code)
	}
}
