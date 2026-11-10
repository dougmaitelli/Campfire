//go:build !production

package integrations

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/url"
)

//go:embed discord-viewer/*
var discordViewer embed.FS

type discordChannel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Failure string `json:"failure"`
}

var discordChannels = []discordChannel{
	{ID: "400", Name: "server-updates"},
	{ID: "401", Name: "read-only"},
	{ID: "402", Name: "other-updates"},
}

// ViewerHandler exposes inspection and failure controls separately from the API.
// It is mounted only by mock-dev and the isolated browser-test executable.
func (d *Discord) ViewerHandler() http.Handler {
	mux := http.NewServeMux()
	for path, asset := range map[string]string{
		"/mock/discord":            "index.html",
		"/mock/discord/{$}":        "index.html",
		"/mock/discord/viewer.css": "viewer.css",
		"/mock/discord/viewer.js":  "viewer.js",
	} {
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, _ *http.Request) {
			content, err := discordViewer.ReadFile("discord-viewer/" + asset)
			if err != nil {
				http.Error(w, "Viewer asset unavailable", http.StatusInternalServerError)
				return
			}

			switch asset {
			case "index.html":
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
			case "viewer.css":
				w.Header().Set("Content-Type", "text/css; charset=utf-8")
			case "viewer.js":
				w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			}

			_, _ = w.Write(content)
		})
	}

	mux.HandleFunc("GET /mock/discord/state", func(w http.ResponseWriter, _ *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()

		channels := append([]discordChannel{}, discordChannels...)
		for index := range channels {
			channels[index].Failure = d.failures[channels[index].ID]
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"token": DiscordToken, "guild": "Campfire Community", "channels": channels, "messages": d.messages})
	})
	mux.HandleFunc("PUT /mock/discord/channels/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Failure string `json:"failure"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input) != nil {
			http.Error(w, "Invalid failure setting", http.StatusBadRequest)
			return
		}

		if r.PathValue("id") != "400" && r.PathValue("id") != "402" {
			http.NotFound(w, r)
			return
		}

		switch input.Failure {
		case "", "forbidden", "rate-limit", "unavailable":
		default:
			http.Error(w, "Unknown failure setting", http.StatusBadRequest)
			return
		}

		d.mu.Lock()
		d.failures[r.PathValue("id")] = input.Failure
		d.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /mock/discord/messages/{id}", func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()

		index := d.findMessage("", r.PathValue("id"))
		if index < 0 {
			http.NotFound(w, r)
			return
		}

		d.deleteMessage(index)
		w.WriteHeader(http.StatusNoContent)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'")

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			// No CORS is offered, and a custom header prevents cross-site form writes.
			origin, err := url.Parse(r.Header.Get("Origin"))
			if r.Header.Get("X-Campfire-Mock") != "discord" || r.Header.Get("Sec-Fetch-Site") == "cross-site" || err != nil || (origin.Host != "" && origin.Host != r.Host) {
				http.Error(w, "Use the local mock viewer", http.StatusForbidden)
				return
			}
		}

		mux.ServeHTTP(w, r)
	})
}
