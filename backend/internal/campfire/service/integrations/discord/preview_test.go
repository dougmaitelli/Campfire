package discord

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestPreviewOmitsEmptyCursorAndPaginates(t *testing.T) {
	for _, size := range []int{0, 200} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			pages := 0

			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				switch r.URL.Path {
				case "/users/@me":
					_ = json.NewEncoder(w).Encode(User{ID: "100", Username: "new-bot", Bot: true})
				case "/users/@me/guilds":
					query := r.URL.Query()
					if query.Get("limit") != "200" || (pages == 0 && query.Has("after")) || (pages == 1 && query.Get("after") != "1199") {
						http.Error(w, "invalid cursor", http.StatusBadRequest)
						return
					}

					page := []Guild{}

					if pages == 0 {
						for i := range size {
							page = append(page, Guild{ID: strconv.Itoa(1000 + i), Name: "Community"})
						}
					}

					pages++
					_ = json.NewEncoder(w).Encode(page)
				default:
					http.NotFound(w, r)
				}
			}))
			defer remote.Close()

			preview, err := NewClient(remote.URL, remote.Client()).Preview(t.Context(), "private-token")
			if err != nil || len(preview.Guilds) != size || preview.InviteURL != BotInviteURL("100", "") {
				t.Fatal("preview failed", preview, err)
			}

			expectedPages := 1
			if size == 200 {
				expectedPages = 2
			}

			if pages != expectedPages {
				t.Fatal("wrong page count", pages)
			}
		})
	}
}
