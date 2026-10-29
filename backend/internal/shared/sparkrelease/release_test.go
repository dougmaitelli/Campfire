package sparkrelease

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVersionComparison(t *testing.T) {
	for _, check := range []struct {
		current, latest string
		newer           bool
	}{
		{"v1.9.9", "v1.10.0", true}, {"1.0.0", "v2.0.0", true}, {"v1.0.0-rc.1", "v1.0.0", true},
		{"v2.0.0", "v1.10.0", false}, {"v1.0.0+build", "v1.0.0", false}, {"dev", "v2.0.0", false}, {"mock-dev", "v2.0.0", false},
		{"v1.0.0", "v1.0.0", false}, {"v1.0.0", "v2.0.0-rc.1", true}, {"v01.0.0", "v2.0.0", false},
		{"v1.0.0-rc.2", "v1.0.0-rc.10", true}, {"v1.0.0-rc.10", "v1.0.0-rc.2", false},
		{"v1.0.0", "v1.0.0-rc.1", false}, {"v1.0.0-beta", "v1.0.0-rc.1", true},
	} {
		if got := Newer(check.current, check.latest); got != check.newer {
			t.Fatalf("%s -> %s: %v", check.current, check.latest, got)
		}
	}
}

func TestVerifiedArchiveExtractsOnlySparkAndRejectsTampering(t *testing.T) {
	for _, scenario := range []string{"valid", "checksum", "symlink", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			var archive bytes.Buffer

			zipped := gzip.NewWriter(&archive)
			writer := tar.NewWriter(zipped)

			filename, kind := "./spark", byte(tar.TypeReg)
			if scenario == "symlink" {
				kind = tar.TypeSymlink
			}

			if scenario == "missing" {
				filename = "../../spark"
			}

			payload := []byte("verified Spark executable")

			header := &tar.Header{Name: filename, Typeflag: kind, Mode: 0755, Size: int64(len(payload))}
			if scenario == "symlink" {
				header.Size = 0
				header.Linkname = "/etc/passwd"
			}

			if err := writer.WriteHeader(header); err != nil {
				t.Fatal(err)
			}

			if header.Size > 0 {
				if _, err := writer.Write(payload); err != nil {
					t.Fatal(err)
				}
			}

			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}

			if err := zipped.Close(); err != nil {
				t.Fatal(err)
			}

			digest := sha256.Sum256(archive.Bytes())

			checksum := fmt.Sprintf("%x  %s\n", digest, archiveName("amd64"))
			if scenario == "checksum" {
				checksum = strings.Repeat("0", 64) + "  " + archiveName("amd64")
			}

			client := &Client{BaseURL: "https://panel.test/spark/downloads", HTTP: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				if !strings.HasPrefix(r.URL.String(), "https://panel.test/spark/downloads/v1.2.3/spark_linux_amd64.tar.gz") {
					t.Fatal("download escaped the panel build", r.URL)
				}

				body := archive.Bytes()
				if strings.HasSuffix(r.URL.Path, ".sha256") {
					body = []byte(checksum)
				}

				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
			})}}
			directory := t.TempDir()

			filename, err := client.Stage(t.Context(), "v1.2.3", "amd64", directory)
			if scenario == "valid" {
				if err != nil {
					t.Fatal(err)
				}

				data, err := os.ReadFile(filename)
				if err != nil || !bytes.Equal(data, payload) {
					t.Fatal("wrong extracted binary", err)
				}

				info, err := os.Stat(filename)
				if err != nil || info.Mode().Perm() != 0755 {
					t.Fatal("binary is not executable", err)
				}
			} else if err == nil {
				t.Fatal("invalid archive accepted")
			}

			items, err := os.ReadDir(directory)
			if err != nil || (scenario != "valid" && len(items) != 0) {
				t.Fatal("failed download left staged files", items, err)
			}
		})
	}
}

func TestPanelBuildUpdates(t *testing.T) {
	for _, check := range []struct {
		current, target string
		available       bool
	}{
		{"dev-abcdef123", "dev-123abcdef", true},
		{"dev-abcdef123", "dev-abcdef123", false},
		{"dev", "dev-123abcdef", true},
		{"dev", "v0.3.0", true},
		{"dev-abcdef123", "v0.3.0", true},
		{"dev-abcdef123", "v0.3.0-rc.1", true},
		{"dev", "dev", false},
		{"dev-abcdef123", "dev", false},
		{"dev-invalid", "v0.3.0", false},
		{"unknown", "v0.3.0", false},
		{"v1.1.0", "v1.0.0", false},
		{"v1.0.0", "v1.1.0", true},
		{"v1.0.0", "dev-abcdef123", false},
		{"dev-abcdef123", "../../escape", false},
	} {
		if got := UpdateAvailable(check.current, check.target); got != check.available {
			t.Fatalf("%s -> %s: %v", check.current, check.target, got)
		}
	}
}

func TestPanelDownloadsRejectRedirectsAndInsecureOrigins(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://other.example/spark", http.StatusFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client().Transport)
	if _, err := client.Stage(t.Context(), "v1.0.0", "amd64", t.TempDir()); err == nil || !strings.Contains(err.Error(), "redirects are not allowed") {
		t.Fatal("package download followed an external redirect", err)
	}

	client.BaseURL = "http://panel.test/spark/downloads"
	if _, err := client.Stage(t.Context(), "v1.0.0", "amd64", t.TempDir()); err == nil {
		t.Fatal("package download allowed HTTP")
	}
}
