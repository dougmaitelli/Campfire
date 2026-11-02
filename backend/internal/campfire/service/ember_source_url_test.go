package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestPelicanYAMLSourceURLSurvivesImportAndUpdates(t *testing.T) {
	const (
		source   = "https://example.com/egg-valheim-bepinex.yaml"
		declared = "https://example.com/egg-valheim-bepinex.json"
	)

	body := strings.Replace(string(eggFixture(t, "valheim-bepinex.yaml")), "update_url: null", "update_url: "+declared, 1)
	client := &http.Client{Transport: definitionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != source {
			t.Fatalf("download used the declared JSON URL instead of the entered YAML URL: %s", r.URL)
		}

		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})}

	preview, err := fetchEmberURL(t.Context(), source, client, ImportOptions{})
	if err != nil || preview.Ember == nil || preview.Ember.UpdateURL != source {
		t.Fatal("URL preview replaced the entered source URL", preview, err)
	}

	store := &importMemoryStore{}
	embers := NewEmberService(store)

	ember, err := embers.ImportDefinition(t.Context(), []byte(preview.Definition), ImportOptions{SourceURL: source, AcknowledgeWarnings: true})
	if err != nil || ember.UpdateURL != source || ember.Origin == nil || ember.Origin.UpdateURL != declared {
		t.Fatal("import lost the entered URL or original metadata", ember, err)
	}

	checks := &memoryEmberUpdates{items: map[string]model.EmberUpdate{}}
	updates := NewEmberUpdateService(embers, checks)

	updates.fetch = func(ctx context.Context, address string, options ImportOptions) (EmberPreview, error) {
		return fetchEmberURL(ctx, address, client, options)
	}
	for range 2 {
		item, preview, err := updates.Check(t.Context(), ember.ID)
		if err != nil || item.Error != "" || item.Available || preview.Ember == nil || preview.Ember.UpdateURL != source {
			t.Fatal("unchanged YAML triggered an update or lost its source URL", item, preview, err)
		}
	}

	// Origin metadata is excluded from the fingerprint, including URL changes.
	body = strings.Replace(body, declared, "https://example.com/another.json", 1)

	item, _, err := updates.Check(t.Context(), ember.ID)
	if err != nil || item.Error != "" || item.Available {
		t.Fatal("upstream URL metadata triggered a false update", item, err)
	}

	body = strings.Replace(body, "name: Valheim BepInEx fixture", "name: Valheim BepInEx fixture v2", 1)

	item, preview, err = updates.Check(t.Context(), ember.ID)
	if err != nil || item.Error != "" || !item.Available || preview.Ember == nil || preview.Ember.UpdateURL != source {
		t.Fatal("real definition update was missed or replaced the entered URL", item, preview, err)
	}

	saved, err := embers.ImportDefinition(t.Context(), []byte(preview.Definition), ImportOptions{ReplaceID: ember.ID, SourceURL: source, AcknowledgeWarnings: true})
	if err != nil || saved.UpdateURL != source {
		t.Fatal("applying the update lost the entered URL", saved, err)
	}

	item, _, err = updates.Check(t.Context(), ember.ID)
	if err != nil || item.Error != "" || item.Available {
		t.Fatal("applied update remained available", item, err)
	}
}
