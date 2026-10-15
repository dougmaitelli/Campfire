// Package webui serves the bundled frontend alongside the authenticated panel API.
package webui

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

var fingerprinted = regexp.MustCompile(`-[A-Za-z0-9_-]{8,}\.[A-Za-z0-9.]+$`)

// Handler mounts the frontend, health endpoint and panel API on the same listener.
func Handler(api http.Handler) http.Handler {
	return newHandler(api, assets())
}

func newHandler(api http.Handler, files fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api", api)
	mux.Handle("/api/", api)
	mux.HandleFunc("/spark", http.NotFound)
	mux.HandleFunc("/spark/", http.NotFound)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}

		if files == nil {
			http.Error(w, "Frontend is not bundled in this development build. Use make dev or make build.", http.StatusServiceUnavailable)
			return
		}

		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}

		if !fs.ValidPath(name) || strings.Contains(name, "\\") {
			http.NotFound(w, r)
			return
		}

		info, err := fs.Stat(files, name)
		if err != nil && path.Ext(name) == "" && !strings.HasPrefix(name, "assets/") {
			name = "index.html"
			info, err = fs.Stat(files, name)
		}

		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-cache")

		if strings.HasPrefix(name, "assets/") && fingerprinted.MatchString(name) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}

		// ServeFileFS canonicalizes index.html to its directory; serve the SPA entry directly.
		if name == "index.html" {
			file, err := files.Open(name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer func() { _ = file.Close() }()

			content, ok := file.(io.ReadSeeker)
			if !ok {
				http.Error(w, "Frontend unavailable.", http.StatusInternalServerError)
				return
			}

			http.ServeContent(w, r, name, info.ModTime(), content)

			return
		}

		http.ServeFileFS(w, r, files, name)
	})

	return mux
}
