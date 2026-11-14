package service

import (
	"encoding/json"
	"testing"
)

func TestNativeEmberRejectsUnknownConfigSettings(t *testing.T) {
	var definition map[string]any

	if err := json.Unmarshal(eggFixture(t, "native.json"), &definition); err != nil {
		t.Fatal(err)
	}

	definition["configs"] = []any{map[string]any{
		"path": "settings.json", "parser": "json", "values": map[string]string{"port": "25565"}, "unexpected": true,
	}}

	data, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}

	report := PreviewEmber(data, ImportOptions{})

	if len(report.Errors) == 0 || report.Ember != nil {
		t.Fatal("unknown configuration setting was accepted")
	}
}
