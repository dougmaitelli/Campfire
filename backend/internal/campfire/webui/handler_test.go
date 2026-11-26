package webui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"campfire/internal/campfire/api"
)

func TestFrontendRoutingAndCacheBoundaries(t *testing.T) {
	files := fstest.MapFS{
		"index.html":                 {Data: []byte("<!doctype html><div id=\"root\"></div>")},
		"assets/app-12345678.js":     {Data: []byte("console.log('Campfire')")},
		"favicon.png":                {Data: []byte("\x89PNG\r\n\x1a\n")},
		"manifest.webmanifest":       {Data: []byte(`{"name":"Campfire","start_url":"/","display":"standalone"}`)},
		"icons/app-192.png":          {Data: []byte("\x89PNG\r\n\x1a\n")},
		"icons/apple-touch-icon.png": {Data: []byte("\x89PNG\r\n\x1a\n")},
		"brand/campfire.svg":         {Data: []byte("<svg></svg>")},
		"install-spark.sh":           {Data: []byte("#!/usr/bin/env bash\nset -euo pipefail\n")},
	}
	handler := HandlerFromFS(api.New(nil, nil, nil).Handler(), files)

	for _, test := range []struct {
		method, url     string
		status          int
		contains, cache string
	}{
		{"GET", "/", 200, "<!doctype html>", "no-cache"},
		{"GET", "/index.html", 200, "<!doctype html>", "no-cache"},
		{"HEAD", "/", 200, "", "no-cache"},
		{"GET", "/servers/example", 200, "<!doctype html>", "no-cache"},
		{"GET", "/status/community", 200, "<!doctype html>", "no-store"},
		{"HEAD", "/status/community", 200, "", "no-store"},
		{"GET", "/assets/app-12345678.js", 200, "console.log", "public, max-age=31536000, immutable"},
		{"GET", "/favicon.png", 200, "PNG", "no-cache"},
		{"GET", "/manifest.webmanifest", 200, `"display":"standalone"`, "no-cache"},
		{"HEAD", "/manifest.webmanifest", 200, "", "no-cache"},
		{"GET", "/icons/app-192.png", 200, "PNG", "no-cache"},
		{"GET", "/icons/apple-touch-icon.png", 200, "PNG", "no-cache"},
		{"GET", "/icons/missing.png", 404, "", ""},
		{"GET", "/service-worker.js", 404, "", ""},
		{"GET", "/brand/campfire.svg", 200, "<svg>", "no-cache"},
		{"GET", "/install-spark.sh", 200, "#!/usr/bin/env bash", "no-cache"},
		{"GET", "/assets/missing.js", 404, "", ""},
		{"GET", "/assets/missing", 404, "", ""},
		{"GET", "/assets", 404, "", ""},
		{"GET", "/missing.css", 404, "", ""},
		{"GET", "/.env", 404, "", ""},
		{"GET", "/api/v1/servers", 503, "authentication", "no-store"},
		{"GET", "/api/unknown", 503, "authentication", "no-store"},
		{"GET", "/spark/enroll", 404, "", ""},
		{"GET", "/healthz", 204, "", "no-store"},
		{"POST", "/", 405, "", ""},
	} {
		t.Run(test.method+test.url, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), test.method, test.url, nil))

			if response.Code != test.status || !strings.Contains(response.Body.String(), test.contains) || response.Header().Get("Cache-Control") != test.cache {
				t.Fatalf("%d %q cache=%q", response.Code, response.Body.String(), response.Header().Get("Cache-Control"))
			}

			if strings.HasPrefix(test.url, "/status/") && (response.Header().Get("X-Robots-Tag") != "noindex, nofollow" || response.Header().Get("Referrer-Policy") != "no-referrer") {
				t.Fatal("public document headers missing", response.Header())
			}

			if test.url == "/manifest.webmanifest" && response.Header().Get("Content-Type") != "application/manifest+json" {
				t.Fatal("incorrect manifest content type", response.Header())
			}

			if test.method == "HEAD" && response.Body.Len() != 0 {
				t.Fatal("HEAD returned frontend bytes")
			}
		})
	}
}

func TestDevelopmentBuildStillServesProtectedAPIAndHealth(t *testing.T) {
	handler := HandlerFromFS(api.New(nil, nil, nil).Handler(), nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), "GET", "/", nil))

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "make dev") {
		t.Fatal("development build falsely advertised bundled assets")
	}
}
