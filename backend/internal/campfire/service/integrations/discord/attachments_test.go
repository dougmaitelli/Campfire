package discord

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"campfire/internal/shared/model"
)

func TestDiscordAttachmentsUseReplayableMultipartAndSafeMentions(t *testing.T) {
	for _, contentType := range []string{"image/png", "image/jpeg", "video/mp4", "video/webm"} {
		t.Run(contentType, func(t *testing.T) {
			data := []byte("retained media bytes")

			var first []byte

			var attempts int

			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++

				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}

				if attempts == 1 {
					first = body
				} else if !bytes.Equal(first, body) {
					t.Error("ambiguous retry changed multipart bytes")
				}

				r.Body = io.NopCloser(bytes.NewReader(body))

				reader, err := r.MultipartReader()
				if err != nil {
					t.Error(err)
					return
				}

				jsonPart, err := reader.NextPart()
				if err != nil || jsonPart.FormName() != "payload_json" {
					t.Error("missing multipart JSON", err)
					return
				}

				var payload struct {
					Nonce    string `json:"nonce"`
					Enforce  bool   `json:"enforce_nonce"`
					Mentions struct {
						Parse []string `json:"parse"`
					} `json:"allowed_mentions"`
				}
				if err := json.NewDecoder(jsonPart).Decode(&payload); err != nil || payload.Nonce != "nonce" || !payload.Enforce || payload.Mentions.Parse == nil || len(payload.Mentions.Parse) != 0 {
					t.Error("attachment lost duplicate/mention protection", payload, err)
				}

				file, err := reader.NextPart()
				if err != nil || file.FormName() != "files[0]" || file.FileName() != `Alex's "clip" file` || file.Header.Get("Content-Type") != contentType {
					t.Error("file metadata did not survive multipart encoding", err)
					return
				}

				stored, err := io.ReadAll(file)
				if err != nil || !bytes.Equal(stored, data) {
					t.Error("attachment bytes changed", err)
				}

				if attempts == 1 {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				_, _ = w.Write([]byte(`{"id":"500","channel_id":"400"}`))
			}))
			defer remote.Close()

			file := Attachment{Media: model.GameMedia{ID: "clip", Name: `Alex's "clip" file`, ContentType: contentType, Size: int64(len(data))}, Data: data}

			id, err := NewClient(remote.URL, remote.Client()).SendEvent(t.Context(), "token", "400", "Server", "nonce", notificationEvent(), file)
			if err != nil || id != "500" || attempts != 2 {
				t.Fatal("attachment retry failed", id, err, attempts)
			}
		})
	}
}

func TestDiscordAttachmentBodyRejectsInvalidAndOversizedFiles(t *testing.T) {
	for _, media := range []model.GameMedia{
		{ID: "file", Name: "unsafe.html", ContentType: "text/html", Size: 1},
		{ID: "file", Name: "bad\nname.png", ContentType: "image/png", Size: 1},
		{ID: "file", Name: "large.mp4", ContentType: "video/mp4", Size: AttachmentFileLimit + 1},
	} {
		if _, err := attachmentBody(struct{}{}, []Attachment{{Media: media, Data: []byte("x")}}); err == nil {
			t.Fatal("invalid attachment accepted", media)
		}
	}
}
