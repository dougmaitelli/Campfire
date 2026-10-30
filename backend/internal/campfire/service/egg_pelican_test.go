package service

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestPelicanValheimBepInExConversion(t *testing.T) {
	data := eggFixture(t, "valheim-bepinex.yaml")
	for _, format := range []string{"yaml", "json"} {
		t.Run(format, func(t *testing.T) {
			if format == "json" {
				var err error

				data, err = definitionDocument(data)
				if err != nil {
					t.Fatal(err)
				}
			}

			report := PreviewEmber(data, ImportOptions{})
			if len(report.Errors) != 0 || report.Ember == nil {
				t.Fatal("Pelican conversion failed", report.Errors)
			}

			ember := report.Ember
			if report.Format != "PLCN_v3" || ember.StartupShell != "bash" || ember.Ready != "Game server connected" || ember.Stop != "^C" || len(ember.Variables) != 17 {
				t.Fatal("lost game process or variable behavior", ember)
			}

			byName := map[string]model.Variable{}
			for _, variable := range ember.Variables {
				byName[variable.Name] = variable
			}

			url := byName["BEPINEX_URL"]
			if !url.Hidden || url.Editable || !url.Required || url.Validation == nil || url.Validation.Format != "url" || url.ValidateValue(url.Default) != nil || url.ValidateValue("not a URL") == nil {
				t.Fatal("URL validation or hidden default was lost", url)
			}

			for name, value := range map[string]string{"BACKUP_INTERVAL": "1800", "BACKUP_COUNT": "4", "BACKUP_SHORTTIME": "7200", "BACKUP_LONGTIME": "43200"} {
				variable := byName[name]
				if variable.Default != value || variable.Hidden || !variable.Editable || variable.ValidateValue("-1") == nil || !strings.Contains(ember.StartupCommands["Default"], "{{"+name+"}}") {
					t.Fatal("game-managed backup option changed", name, variable)
				}
			}

			if !strings.Contains(ember.StartupCommands["Default"], `> >(sed -uE "{{CONSOLE_FILTER}}")`) || ember.Installer == nil || !strings.Contains(ember.Installer.Script, `"$BEPINEX_URL" "$V_MODPACK"`) {
				t.Fatal("Bash startup or installer environment references were changed")
			}

			encoded, err := json.Marshal(ember)
			if err != nil {
				t.Fatal(err)
			}

			native := PreviewEmber(encoded, ImportOptions{})
			if len(native.Errors) != 0 || native.Ember == nil || native.Ember.Variables[5].Validation == nil || native.Ember.Variables[5].Validation.Format != "url" || native.Ember.Variables[5].ValidateValue("invalid") == nil {
				t.Fatal("native definition lost URL validation", native.Errors)
			}
		})
	}
}

func TestEggEmptyListRepresentationsRemainStrict(t *testing.T) {
	for _, field := range []string{"tags", "features", "images", "variables"} {
		for _, value := range []string{`[]`, `{}`, `null`, `{"unexpected":"value"}`, `""`, `[42]`} {
			t.Run(field+"/"+value, func(t *testing.T) {
				document := map[string]any{
					"meta": map[string]string{"version": "PLCN_v3"}, "name": "Empty lists",
					"docker_images":    map[string]string{"Alpine": "alpine:3.22"},
					"startup_commands": map[string]string{"Default": "sleep 3600"},
					field:              json.RawMessage(value),
				}

				data, err := json.Marshal(document)
				if err != nil {
					t.Fatal(err)
				}

				report := PreviewEmber(data, ImportOptions{})

				valid := slices.Contains([]string{`[]`, `{}`, `null`}, value)
				if valid && (report.Ember == nil || len(report.Errors) > 0) || !valid && (report.Ember != nil || len(report.Errors) == 0) {
					t.Fatal("empty-list handling weakened schema validation", field, value, report.Errors)
				}
			})
		}
	}

	data := []byte(`{"meta":{"version":"PLCN_v3"},"name":"Strict variable","docker_images":{"Alpine":"alpine:3.22"},"startup_commands":{"Default":"sleep 3600"},"variables":[{"env_variable":"NAME","default_value":"value","user_editable":true,"unexpected":true}]}`)
	if report := PreviewEmber(data, ImportOptions{}); report.Ember != nil || len(report.Errors) == 0 {
		t.Fatal("custom list decoder allowed unknown variable fields")
	}
}
