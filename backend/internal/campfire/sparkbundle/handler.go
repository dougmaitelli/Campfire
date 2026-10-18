// Package sparkbundle serves Linux Spark packages built alongside this panel.
package sparkbundle

import (
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"campfire/internal/shared/buildinfo"
	"campfire/internal/shared/protocol"
)

// Available reports whether both supported architectures were bundled.
func Available() bool {
	return available(assets())
}

func available(files fs.FS) bool {
	if files == nil {
		return false
	}

	for _, arch := range []string{"amd64", "arm64"} {
		for _, suffix := range []string{".tar.gz", ".tar.gz.sha256"} {
			if _, err := fs.Stat(files, "spark_linux_"+arch+suffix); err != nil {
				return false
			}
		}
	}

	return true
}

// Handler expects its mount prefix to be stripped. Packages are pinned to this build.
func Handler() http.Handler {
	return newHandler(assets(), buildinfo.SparkVersion())
}

func newHandler(files fs.FS, build string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}

		if !available(files) {
			http.Error(w, "Spark packages are not bundled. Build Campfire with make build.", http.StatusServiceUnavailable)
			return
		}

		if r.URL.Path == "version" || r.URL.Path == "/version" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")

			if r.Method != http.MethodHead {
				_, _ = w.Write([]byte(build + "\n"))
			}

			return
		}

		version, name, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if !ok || version != build {
			http.NotFound(w, r)
			return
		}

		switch name {
		case "spark_linux_amd64.tar.gz", "spark_linux_arm64.tar.gz", "spark_linux_amd64.tar.gz.sha256", "spark_linux_arm64.tar.gz.sha256":
			if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(protocol.SparkUpdateTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
				http.Error(w, "Spark download unavailable.", http.StatusInternalServerError)
				return
			}

			http.ServeFileFS(w, r, files, name)
		default:
			http.NotFound(w, r)
		}
	})
}
