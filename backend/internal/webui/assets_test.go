//go:build production

package webui

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProductionBundleIncludesEntrypointAndAssets(t *testing.T) {
	files := assets()
	index, err := fs.ReadFile(files, "index.html")
	if err != nil || !strings.Contains(string(index), "/assets/") {
		t.Fatal("production binary is missing its compiled frontend", err)
	}

	entries, err := fs.Glob(files, "assets/*.js")
	if err != nil || len(entries) == 0 {
		t.Fatal("production bundle has no JavaScript", err)
	}

	handler := Handler(http.NotFoundHandler())
	for _, name := range append([]string{"index.html"}, entries...) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), "GET", "/"+name, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatal("embedded frontend cannot be served", name, response.Code)
		}
	}
}
