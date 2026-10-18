package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

type definitionTransport func(*http.Request) (*http.Response, error)

func (f definitionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDefinitionURLPreview(t *testing.T) {
	definition := `{"schema":1,"name":"Game","description":"","images":{"image":"alpine:3.22"},"startup":"sleep 3600","work_dir":"/home/container","user":"1000:1000","variables":[]}`
	for _, test := range []struct {
		name   string
		body   string
		status int
		valid  bool
	}{
		{"valid", definition, http.StatusOK, true},
		{"invalid JSON", "<html>not JSON</html>", http.StatusOK, false},
		{"trailing JSON", definition + " {}", http.StatusOK, false},
		{"invalid Ember", `{"schema":2}`, http.StatusOK, false},
		{"oversized", strings.Repeat("x", maxEmberBytes+1), http.StatusOK, false},
		{"HTTP error", definition, http.StatusNotFound, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: definitionTransport(func(r *http.Request) (*http.Response, error) {
				if !strings.Contains(r.Header.Get("Accept"), "application/json") || r.URL.Scheme != "https" {
					t.Fatal("missing JSON or HTTPS request")
				}

				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body)), Header: make(http.Header), Request: r}, nil
			})}

			ember, err := fetchEmberURL(context.Background(), "https://example.com/game.json", client, ImportOptions{})
			if test.valid {
				if err != nil || ember.Ember == nil || ember.Ember.Name != "Game" || ember.Ember.ID != "" {
					t.Fatalf("preview failed: %+v %v", ember, err)
				}
			} else if !errors.Is(err, ErrInvalidEmber) && len(ember.Errors) == 0 {
				t.Fatalf("expected invalid definition, got %v", err)
			}
		})
	}
}

func TestURLImportsPreserveUpdateURLThroughPreviewAndSave(t *testing.T) {
	const source = "https://example.com/import.json"
	for _, test := range []struct {
		name     string
		fixture  string
		declared string
		want     string
	}{
		{"native fallback", "native.json", "", source},
		{"native declared", "native.json", "https://example.com/native.json", "https://example.com/native.json"},
		{"egg fallback", "ptdl-v2.json", "", source},
		{"egg declared", "ptdl-v2.json", "https://example.com/egg.json", "https://example.com/egg.json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var definition map[string]any
			if err := json.Unmarshal(eggFixture(t, test.fixture), &definition); err != nil {
				t.Fatal(err)
			}

			if meta, ok := definition["meta"].(map[string]any); ok {
				meta["update_url"] = test.declared
			} else if test.declared != "" {
				definition["updateUrl"] = test.declared
			}

			data, err := json.Marshal(definition)
			if err != nil {
				t.Fatal(err)
			}

			client := &http.Client{Transport: definitionTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header), Request: r}, nil
			})}

			preview, err := fetchEmberURL(t.Context(), source, client, ImportOptions{})
			if err != nil || preview.Ember == nil || preview.Ember.UpdateURL != test.want {
				t.Fatal("URL preview lost update URL", preview, err)
			}

			store := &importMemoryStore{}

			saved, err := NewEmberService(store).ImportDefinition(t.Context(), []byte(preview.Definition), ImportOptions{SourceURL: source, AcknowledgeWarnings: true})
			if err != nil || saved.UpdateURL != test.want || len(store.saved) != 1 || store.saved[0].UpdateURL != test.want {
				t.Fatal("import lost update URL", saved, err)
			}
		})
	}
}

func TestDefinitionURLRestrictions(t *testing.T) {
	for _, address := range []string{
		"http://example.com/game.json", "file:///etc/passwd", "https://user:password@example.com/game.json",
		"https://example.com/game.json#fragment", "https://127.0.0.1/game.json",
		"https://10.1.2.3/game.json", "https://[::1]/game.json", "https://[::ffff:127.0.0.1]/game.json",
		"https://169.254.169.254/game.json", "https://100.64.1.2/game.json",
	} {
		target, err := url.Parse(address)
		if err != nil {
			t.Fatal(err)
		}

		if err := validateEmberURL(target); !errors.Is(err, ErrInvalidEmber) {
			t.Errorf("allowed invalid URL %s", address)
		}
	}

	for _, address := range []string{"127.0.0.1", "10.0.0.1", "192.168.0.1", "172.16.0.1", "169.254.1.1", "::", "::1", "fc00::1", "fe80::1", "::ffff:192.168.0.1", "224.0.0.1"} {
		if publicAddress(netip.MustParseAddr(address)) {
			t.Errorf("allowed non-public address %s", address)
		}
	}

	if !publicAddress(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("rejected public address")
	}
}

func TestDefinitionFetchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &http.Client{Transport: definitionTransport(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })}
	if _, err := fetchEmberURL(ctx, "https://example.com/game.json", client, ImportOptions{}); !errors.Is(err, ErrInvalidEmber) {
		t.Fatalf("canceled fetch: %v", err)
	}
}
