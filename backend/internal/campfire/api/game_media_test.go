package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestGameMediaDownloadsRequireServerAccessAndSupportRanges(t *testing.T) {
	f := newAccessFixture(t)
	user, member := f.user("game-media", database.UserRole)
	_, _, input := serverFixture(t, f.admin)
	response := request(f.admin, "POST", "/api/v1/servers", configJSON(t, input))

	var server model.Server
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &server) != nil {
		t.Fatal(response.Code, response.Body)
	}

	frame := protocol.GameFrame{ServerID: server.ID, Publication: model.GamePublication{Version: 1, Event: &model.GameEvent{ID: "screenshot-event", Type: "game.announcement", Title: "Screenshot", OccurredAt: time.Now()}}}
	if err := f.games.Receive(t.Context(), server.SparkID, frame); err != nil {
		t.Fatal(err)
	}

	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}

	media := model.GameMedia{ID: "shot", Name: "world.png", ContentType: "image/png"}
	if err := f.games.Media.Receive(t.Context(), server.SparkID, server.ID, "screenshot-event", media, bytes.NewReader(imageData.Bytes())); err != nil {
		t.Fatal(err)
	}

	page, err := f.games.Events.Store.List(t.Context(), server.ID, repository.GameEventQuery{Limit: 10})
	if err != nil || len(page.Items[0].Attachments) != 1 {
		t.Fatal(page, err)
	}

	endpoint := fmt.Sprintf("/api/v1/servers/%s/events/%d/media/shot", server.ID, page.Items[0].ID)
	expectAccess(t, member, "GET", endpoint, "", http.StatusNotFound)
	expectAccess(t, f.admin, "POST", "/api/v1/servers/"+server.ID+"/members", `{"username":"game-media","role_id":5}`, http.StatusOK)

	response = request(member, "GET", endpoint, "")
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), imageData.Bytes()) || response.Header().Get("Content-Type") != "image/png" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal(response.Code, response.Header())
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, nil)
	req.Header.Set("Range", "bytes=0-7")

	response = httptest.NewRecorder()
	f.admin.ServeHTTP(response, req)

	if response.Code != http.StatusPartialContent || !bytes.Equal(response.Body.Bytes(), imageData.Bytes()[:8]) {
		t.Fatal("range failed", response.Code, response.Body)
	}

	response = request(member, "GET", endpoint+"?download=1", "")
	if response.Header().Get("Content-Disposition") != "attachment; filename=world.png" {
		t.Fatal(response.Header())
	}

	expectAccess(t, f.admin, "DELETE", "/api/v1/servers/"+server.ID+"/members/"+fmt.Sprint(user.ID), "", http.StatusNoContent)
	expectAccess(t, member, "GET", endpoint, "", http.StatusNotFound)
}
