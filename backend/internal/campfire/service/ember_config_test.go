package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNativeEmberDropsRetiredConfigStringKeys(t *testing.T) {
	var definition map[string]any

	if err := json.Unmarshal(eggFixture(t, "native.json"), &definition); err != nil {
		t.Fatal(err)
	}

	definition["configs"] = []any{map[string]any{
		"path": "settings.json", "parser": "json", "values": map[string]string{"port": "25565"}, "strings": []string{"port"},
	}}

	data, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}

	report := PreviewEmber(data, ImportOptions{})

	if len(report.Errors) != 0 || report.Ember == nil {
		t.Fatalf("legacy definition was rejected: %+v", report.Errors)
	}

	normalized, err := json.Marshal(report.Ember)

	if err != nil || strings.Contains(string(normalized), `"strings"`) {
		t.Fatalf("retired configuration was retained: %s, %v", normalized, err)
	}
}
