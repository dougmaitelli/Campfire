package service

import (
	"encoding/json"
	"testing"
)

func TestExtendedEggBehaviorConversionAndRemainingBlocks(t *testing.T) {
	report := PreviewEmber(eggFixture(t, "extended.json"), ImportOptions{})
	if report.Ember == nil || len(report.Errors) > 0 {
		t.Fatal(report.Errors)
	}

	ember := report.Ember
	if ember.Stop != "quit {{STOP_REASON}}" || ember.Readiness == nil || !ember.Readiness.StripANSI || len(ember.Readiness.Any) != 2 || !ember.Readiness.Any[1].Regex {
		t.Fatal("lost extended process behavior", ember)
	}

	for _, config := range ember.Configs {
		if !config.Create {
			t.Fatal("missing-file behavior lost", config)
		}
	}

	for _, change := range []func(map[string]any){
		func(config map[string]any) { config["startup"] = map[string]any{"done": "/(?=Ready)Ready/"} },
		func(config map[string]any) {
			config["startup"] = map[string]any{"done": []string{"Ready"}, "userInteraction": []int{1}}
		},
		func(config map[string]any) {
			config["files"] = map[string]any{"settings.xml": map[string]any{"parser": "xml", "find": map[string]string{"server.port": "25565"}}}
		},
	} {
		var document map[string]any
		if err := json.Unmarshal(eggFixture(t, "extended.json"), &document); err != nil {
			t.Fatal(err)
		}

		config, ok := document["config"].(map[string]any)
		if !ok {
			t.Fatal("config object missing")
		}

		change(config)

		data, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}

		if result := PreviewEmber(data, ImportOptions{}); result.Ember != nil || len(result.Errors) == 0 {
			t.Fatal("unsupported behavior accepted", result)
		}
	}
}

func TestEggStopSignals(t *testing.T) {
	for _, stop := range []string{"^SIGABRT", "^SIGKILL", "^SIGQUIT", "^sigusr1", "^SIGRTMAX-1", "^INVALID"} {
		t.Run(stop, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(eggFixture(t, "extended.json"), &document); err != nil {
				t.Fatal(err)
			}

			config, ok := document["config"].(map[string]any)
			if !ok {
				t.Fatal("config object missing")
			}

			config["stop"] = stop

			data, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}

			result := PreviewEmber(data, ImportOptions{})
			if stop == "^INVALID" {
				if result.Ember != nil || len(result.Errors) == 0 {
					t.Fatal("invalid signal accepted", result)
				}
			} else if result.Ember == nil || len(result.Errors) != 0 || result.Ember.Stop != stop {
				t.Fatal("signal import failed", result)
			}
		})
	}
}
