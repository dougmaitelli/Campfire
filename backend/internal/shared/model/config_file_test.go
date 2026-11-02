package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfigFileDiscardsLegacyStringKeys(t *testing.T) {
	var config ConfigFile

	if err := json.Unmarshal([]byte(`{"path":"settings.json","parser":"json","values":{"port":"25565"},"strings":["port"],"create":true}`), &config); err != nil {
		t.Fatal(err)
	}

	if config.Path != "settings.json" || config.Parser != ConfigParserJSON || config.Values["port"] != "25565" || !config.Create {
		t.Fatalf("configuration fields changed: %#v", config)
	}

	data, err := json.Marshal(config)

	if err != nil || strings.Contains(string(data), `"strings"`) {
		t.Fatalf("retired setting was serialized: %s, %v", data, err)
	}

	if err := json.Unmarshal([]byte(`{"path":"settings.json","parser":"json","values":{},"unexpected":true}`), &config); err == nil {
		t.Fatal("unknown configuration fields must remain invalid")
	}
}
