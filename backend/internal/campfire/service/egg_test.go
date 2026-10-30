package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func eggFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", "eggs", name))
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestEggConsolePromptsDoNotBlockImport(t *testing.T) {
	var egg map[string]any
	if err := json.Unmarshal(eggFixture(t, "ptdl-v2.json"), &egg); err != nil {
		t.Fatal(err)
	}

	egg["features"] = []string{"eula", "java_version", "pid_limit"}

	config, ok := egg["config"].(map[string]any)
	if !ok {
		t.Fatal("config object missing")
	}

	config["startup"] = `{"done":")! For help, type ","userInteraction":["Go to eula.txt for more info."]}`

	data, err := json.Marshal(egg)
	if err != nil {
		t.Fatal(err)
	}

	report := PreviewEmber(data, ImportOptions{})
	if len(report.Errors) != 0 || report.Ember == nil {
		t.Fatalf("console prompts blocked conversion: %+v", report.Errors)
	}

	if report.Ember.Ready != ")! For help, type " {
		t.Fatal("readiness marker was changed")
	}

	for _, field := range []string{"features.eula", "features.java_version", "ports"} {
		if !slices.ContainsFunc(report.Warnings, func(issue ImportIssue) bool {
			return issue.Field == field && !issue.Informational
		}) {
			t.Fatalf("missing actionable warning for %s", field)
		}
	}

	if !slices.ContainsFunc(report.Warnings, func(issue ImportIssue) bool {
		return issue.Field == "config.startup.userInteraction" && issue.Informational
	}) {
		t.Fatal("missing informational console notice")
	}

	config["startup"] = `{"userInteraction":[{"unexpected":true}]}`

	data, err = json.Marshal(egg)
	if err != nil {
		t.Fatal(err)
	}

	if len(PreviewEmber(data, ImportOptions{}).Errors) == 0 {
		t.Fatal("malformed prompt marker accepted")
	}
}

func TestEggCompatibilityFixtures(t *testing.T) {
	for _, test := range []struct {
		file, format string
	}{
		{"native.json", "ember"}, {"ptdl-v1.json", "PTDL_v1"}, {"ptdl-v2.json", "PTDL_v2"},
		{"plcn-v1.json", "PLCN_v1"}, {"plcn-v2.json", "PLCN_v2"}, {"plcn-v3.json", "PLCN_v3"}, {"plcn-v3.yaml", "PLCN_v3"},
		{"extended.json", "PTDL_v2"}, {"valheim.json", "PTDL_v2"},
		{"valheim-bepinex.yaml", "PLCN_v3"},
	} {
		t.Run(test.file, func(t *testing.T) {
			data := eggFixture(t, test.file)

			report := PreviewEmber(data, ImportOptions{})
			if len(report.Errors) > 0 || report.Ember == nil {
				t.Fatalf("fixture conversion failed: %+v", report.Errors)
			}

			ember := *report.Ember
			if report.Format != test.format || ember.Schema != 1 || ember.ID != "" {
				t.Fatal("incorrect format/revision", report)
			}

			if err := ember.Validate(); err != nil {
				t.Fatal(err)
			}

			if test.format != "ember" && (len(report.Warnings) < 2 || ember.Origin == nil || ember.Origin.Format != test.format || len(ember.Origin.Warnings) != len(report.Warnings)) {
				t.Fatal("missing compatibility information", report)
			}

			encoded, err := json.Marshal(ember)
			if err != nil {
				t.Fatal(err)
			}

			native := PreviewEmber(encoded, ImportOptions{})
			if native.Ember == nil || len(native.Errors) > 0 {
				t.Fatal("converted definition cannot be read as native Ember", native.Errors)
			}
		})
	}

	report := PreviewEmber(eggFixture(t, "ptdl-v2.json"), ImportOptions{})

	ember := report.Ember
	if ember.Startup != "java -Xmx{{SERVER_MEMORY}}M -jar {{SERVER_JARFILE}}" || ember.Configs[0].Values["server-port"] != "{{SERVER_PORT}}" || ember.Configs[0].Values["max-players"] != "{{MAX_PLAYERS}}" || ember.Ready != "Done (" || ember.Installer.Entrypoint != "ash" {
		t.Fatal("Pterodactyl semantics not retained", ember)
	}

	if ember.Variables[1].ValidateValue("129") == nil || ember.Variables[1].ValidateValue("20") != nil {
		t.Fatal("numeric validation not retained")
	}

	if ember.Variables[0].ValidateValue("server.jar") != nil || ember.Variables[0].ValidateValue("server.sh") == nil {
		t.Fatal("regex validation not retained")
	}

	yaml := PreviewEmber(eggFixture(t, "plcn-v3.yaml"), ImportOptions{})
	if yaml.Ember.Variables[0].Default != "true" || yaml.Ember.Installer.Script != "#!/bin/sh\nprintf '{}\\n' > /mnt/server/settings.json\n" {
		t.Fatal("YAML scalar or literal script changed", yaml.Ember)
	}
}

func TestEggUnsupportedFixtureAndStartupSelection(t *testing.T) {
	report := PreviewEmber(eggFixture(t, "unsupported.json"), ImportOptions{})

	fields := make([]string, 0, len(report.Errors))
	for _, issue := range report.Errors {
		fields = append(fields, issue.Field)
	}

	for _, field := range []string{"file_denylist", "variables[0].rules", "startup", "config.logs"} {
		if !slices.Contains(fields, field) {
			t.Fatal("unsupported behavior was not reported", field, report.Errors)
		}
	}

	if report.Ember != nil {
		t.Fatal("unsupported Egg exposed a usable conversion")
	}

	data := eggFixture(t, "plcn-v3.json")

	report = PreviewEmber(data, ImportOptions{})
	if report.Ember == nil || len(report.Errors) != 0 || len(report.Ember.StartupCommands) != 2 || report.Ember.StartupCommands["Debug"] != "./server --debug" || report.Ember.Startup != "" {
		t.Fatal("named startup commands were not preserved", report)
	}
}

func TestEggDocumentAmbiguitiesAndUnsupportedInputs(t *testing.T) {
	cases := []string{
		`{}`, `[]`, `{"schema":1,"schema":2}`, `{"meta":{"version":"PTDL_v99"}}`,
		`{"meta":{"version":"PLCN_v3"},"schema":1}`, `{"meta":{"version":"PTDL_v2"}} {}`,
		"meta: {version: PLCN_v3}\nmeta: {}", "meta: &egg {version: PLCN_v3}\nconfig: *egg",
		"meta: {version: PLCN_v3}\n---\nname: second", "meta: {version: PLCN_v3}\nvalue: .inf",
		"meta: {version: PLCN_v3}\nvalue: !custom foo", strings.Repeat("[", 80) + "0" + strings.Repeat("]", 80), strings.Repeat("x", maxEmberBytes+1),
	}
	for _, source := range cases {
		report := PreviewEmber([]byte(source), ImportOptions{})
		if report.Ember != nil || len(report.Errors) == 0 {
			t.Fatalf("accepted unsupported/ambiguous input %.80q", source)
		}
	}
}

func TestEggRejectsRuntimeFeaturesAndUnknownFields(t *testing.T) {
	original := eggFixture(t, "ptdl-v2.json")
	for _, test := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"unknown field", func(d map[string]any) { d["host_network"] = true }},
		{"privileged", func(d map[string]any) { d["script_is_privileged"] = true }},
		{"mount", func(d map[string]any) { d["mounts"] = []string{"/etc"} }},
		{"feature", func(d map[string]any) { d["features"] = []string{"unknown-feature"} }},
		{"inherited", func(d map[string]any) { d["config_from"] = 42 }},
		{"outgoing IP", func(d map[string]any) { d["force_outgoing_ip"] = true }},
		{"traversal", func(d map[string]any) {
			d["config"] = map[string]any{"files": map[string]any{"../escape": map[string]any{"parser": "properties", "find": map[string]string{"port": "25565"}}}}
		}},
		{"invalid JSON path", func(d map[string]any) {
			d["config"] = map[string]any{"files": map[string]any{"settings.json": map[string]any{"parser": "json", "find": map[string]string{"nested..port": "25565"}}}}
		}},
		{"conditional", func(d map[string]any) {
			d["config"] = map[string]any{"files": map[string]any{"server.properties": map[string]any{"parser": "properties", "find": map[string]any{"port": map[string]string{"old": "new"}}}}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(original, &object); err != nil {
				t.Fatal(err)
			}

			test.change(object)

			data, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}

			report := PreviewEmber(data, ImportOptions{})
			if report.Ember != nil || len(report.Errors) == 0 {
				t.Fatal("unsupported behavior accepted", report)
			}
		})
	}
}

type importMemoryStore struct{ saved []model.Ember }

func (s *importMemoryStore) Replace(_ context.Context, e model.Ember) error {
	for i := range s.saved {
		if s.saved[i].ID == e.ID {
			s.saved[i] = e
			return nil
		}
	}

	return ErrEmberNotFound
}

func (s *importMemoryStore) Delete(_ context.Context, id string) error {
	for index, ember := range s.saved {
		if ember.ID == id {
			s.saved = slices.Delete(s.saved, index, index+1)
			return nil
		}
	}

	return ErrEmberNotFound
}

func (s *importMemoryStore) Create(_ context.Context, e model.Ember) error {
	s.saved = append(s.saved, e)
	return nil
}
func (s *importMemoryStore) List(context.Context) ([]model.Ember, error) { return s.saved, nil }
func (s *importMemoryStore) Get(_ context.Context, id string) (model.Ember, error) {
	for _, e := range s.saved {
		if e.ID == id {
			return e, nil
		}
	}

	return model.Ember{}, ErrEmberNotFound
}

func TestEggImportRequiresReviewAndPinsIndependentRevisions(t *testing.T) {
	store := &importMemoryStore{}
	service := NewEmberService(store)

	data := eggFixture(t, "ptdl-v2.json")
	if _, err := service.ImportDefinition(context.Background(), data, ImportOptions{}); !errors.Is(err, ErrInvalidEmber) || len(store.saved) != 0 {
		t.Fatal("warnings were silently ignored", err)
	}

	first, err := service.ImportDefinition(context.Background(), data, ImportOptions{AcknowledgeWarnings: true})
	if err != nil {
		t.Fatal(err)
	}

	second, err := service.ImportDefinition(context.Background(), data, ImportOptions{AcknowledgeWarnings: true})
	if err != nil || first.ID == second.ID || first.ID == "" || len(store.saved) != 2 {
		t.Fatal("imports did not create independent revisions", err)
	}

	if _, err := service.ImportDefinition(context.Background(), eggFixture(t, "unsupported.json"), ImportOptions{AcknowledgeWarnings: true}); !errors.Is(err, ErrInvalidEmber) || len(store.saved) != 2 {
		t.Fatal("acknowledgement bypassed unsupported features", err)
	}
}

func TestEggURLUsesSameConversionAndReview(t *testing.T) {
	data := eggFixture(t, "plcn-v3.yaml")
	client := &http.Client{Transport: definitionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header), Request: r}, nil
	})}

	report, err := fetchEmberURL(context.Background(), "https://example.com/egg.yaml", client, ImportOptions{})
	if err != nil || report.Ember == nil || report.Format != "PLCN_v3" || report.Definition != string(data) || len(report.Warnings) == 0 {
		t.Fatal("URL import bypassed conversion", report, err)
	}
}
