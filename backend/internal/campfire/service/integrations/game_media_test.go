package integrations

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	fixture "campfire/internal/campfire/devfixture/integrations"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/shared/model"
)

func TestGameMediaDeliversLateUploadsOnceThroughRealDiscordTransport(t *testing.T) {
	remote := fixture.NewDiscord()

	db, setup, _, _ := deliveryService(t, remote.Handler())
	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "first").Update("game_integration", true).Error; err != nil {
		t.Fatal(err)
	}

	ctx, now := t.Context(), time.Now()

	event := model.GameEvent{ID: "late-media", Type: "game.capture", Title: "World captured", OccurredAt: now, Data: map[string]any{"player": "Alex"}, FieldLabels: model.GameFieldLabels{"player": "Player name"}}
	if err := repository.NewGameEventRepository(db.ORM()).Append(ctx, "node", "first", event, now, time.Hour, 20); err != nil {
		t.Fatal(err)
	}

	if err := setup.delivery.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	mediaStore := repository.NewGameMediaRepository(db.ORM())

	for _, contentType := range []string{"image/png", "video/mp4", "video/webm"} {
		data := []byte(contentType)

		media := model.GameMedia{ID: "upload-" + contentType[6:], Name: "capture", ContentType: contentType, Size: int64(len(data))}
		if err := mediaStore.Append(ctx, "node", "first", event.ID, media, data); err != nil {
			t.Fatal(err)
		}

		if err := setup.delivery.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}

		if err := mediaStore.Append(ctx, "node", "first", event.ID, media, data); err != nil {
			t.Fatal(err)
		}
	}

	if err := setup.delivery.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	viewer := remote.ViewerHandler()
	state := httptest.NewRecorder()
	viewer.ServeHTTP(state, httptest.NewRequestWithContext(ctx, http.MethodGet, "/mock/discord/state", nil))

	var snapshot struct {
		Messages []struct {
			ID          string `json:"id"`
			Attachments []struct {
				URL         string `json:"url"`
				ContentType string `json:"content_type"`
			} `json:"attachments"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(state.Body.Bytes(), &snapshot); err != nil || len(snapshot.Messages) != 4 {
		t.Fatal("late media or retry duplicated/lost a message", state.Body.String(), err)
	}

	for _, message := range snapshot.Messages[1:] {
		if len(message.Attachments) != 1 {
			t.Fatal("attachment missing from provider message", message)
		}

		file := message.Attachments[0]
		response := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(ctx, http.MethodGet, file.URL, nil)
		request.Header.Set("Range", "bytes=0-3")
		viewer.ServeHTTP(response, request)

		if response.Code != http.StatusPartialContent || !bytes.Equal(response.Body.Bytes(), []byte(file.ContentType)[:4]) {
			t.Fatal("mock download lost range support or attachment bytes", response.Code, response.Body.String())
		}

		remove := httptest.NewRequestWithContext(ctx, http.MethodDelete, "/mock/discord/messages/"+message.ID, nil)
		remove.Header.Set("X-Campfire-Mock", "discord")
		viewer.ServeHTTP(httptest.NewRecorder(), remove)
		missing := httptest.NewRecorder()
		viewer.ServeHTTP(missing, httptest.NewRequestWithContext(ctx, http.MethodGet, file.URL, nil))

		if missing.Code != http.StatusNotFound {
			t.Fatal("deleted mock message retained downloadable bytes")
		}
	}

	var messages []database.IntegrationMessage
	if err := db.ORM().Find(&messages).Error; err != nil || len(messages) != 4 {
		t.Fatal("upload retries duplicated durable work", len(messages), err)
	}

	for _, message := range messages {
		if message.State != model.DeliveryDelivered {
			t.Fatal("media message not acknowledged", message.State, message.Error)
		}
	}
}

func TestGameMediaExpiredAndOversizedUploadsExplainMissingAttachments(t *testing.T) {
	for _, size := range []int64{10, 21 << 20} {
		remote := fixture.NewDiscord()
		db, setup, store, _ := deliveryService(t, remote.Handler())

		event := core.Event{ID: "media:missing", ServerID: "first", Category: core.GameEvents, Type: "game.capture", Title: "Capture", Data: map[string]any{}, Attachment: &core.EventAttachment{GameEventID: 123, Media: model.GameMedia{ID: "clip", Name: "clip.mp4", ContentType: "video/mp4", Size: size}}}
		if err := store.RouteEvent(t.Context(), event, time.Now()); err != nil {
			t.Fatal(err)
		}

		if err := setup.delivery.RunOnce(t.Context()); err != nil {
			t.Fatal(err)
		}

		var message database.IntegrationMessage
		if err := db.ORM().Take(&message).Error; err != nil || message.State != model.DeliveryDelivered {
			t.Fatal("unavailable media was retried indefinitely", message.State, err)
		}

		state := httptest.NewRecorder()
		remote.ViewerHandler().ServeHTTP(state, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/mock/discord/state", nil))

		if !bytes.Contains(state.Body.Bytes(), []byte("clip.mp4")) || bytes.Contains(state.Body.Bytes(), []byte(`"attachments"`)) {
			t.Fatal("missing-media explanation was lost", state.Body.String())
		}
	}
}
