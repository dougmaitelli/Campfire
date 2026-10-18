package sparkbundle

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBuildPinnedDownloads(t *testing.T) {
	files := fstest.MapFS{}
	for _, arch := range []string{"amd64", "arm64"} {
		files["spark_linux_"+arch+".tar.gz"] = &fstest.MapFile{Data: []byte(arch + " binary")}
		files["spark_linux_"+arch+".tar.gz.sha256"] = &fstest.MapFile{Data: []byte(arch + " checksum")}
	}

	handler := newHandler(files, "dev-abcdef123")

	for _, check := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/version", "dev-abcdef123\n", 200},
		{"HEAD", "/version", "", 200},
		{"GET", "/dev-abcdef123/spark_linux_amd64.tar.gz", "amd64 binary", 200},
		{"GET", "/dev-abcdef123/spark_linux_arm64.tar.gz.sha256", "arm64 checksum", 200},
		{"HEAD", "/dev-abcdef123/spark_linux_arm64.tar.gz", "", 200},
		{"GET", "/dev-987654321/spark_linux_amd64.tar.gz", "404", 404},
		{"GET", "/dev-abcdef123/spark_linux_386.tar.gz", "404", 404},
		{"GET", "/dev-abcdef123/../secret", "404", 404},
		{"POST", "/version", "", 405},
	} {
		t.Run(check.method+check.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), check.method, check.path, nil))

			if response.Code != check.status || !strings.Contains(response.Body.String(), check.body) || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(response.Code, response.Body.String(), response.Header())
			}

			if check.method == "HEAD" && response.Body.Len() != 0 {
				t.Fatal("HEAD returned package bytes")
			}
		})
	}
}

func TestMissingPackagesFailExplicitly(t *testing.T) {
	response := httptest.NewRecorder()
	newHandler(nil, "dev").ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/version", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatal(response.Code)
	}
}
