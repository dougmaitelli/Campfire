package service

import (
	"encoding/json"
	"testing"

	"campfire/internal/shared/model"
)

func TestValheimHiddenVariablesAndBashStartup(t *testing.T) {
	report := PreviewEmber(eggFixture(t, "valheim.json"), ImportOptions{})
	if report.Ember == nil || len(report.Errors) != 0 {
		t.Fatalf("Valheim conversion failed: %+v", report.Errors)
	}
	ember := report.Ember
	if ember.StartupShell != "bash" || ember.Stop != "^C" || ember.Ready != "DungeonDB Start" {
		t.Fatalf("lost process behavior: %+v", ember)
	}
	hidden := map[string]string{"LD_LIBRARY_PATH": "./linux64", "STOP": "kill -2 $!; wait;", "CONSOLE_FILTER": `/^\(Filename:.*Line:[[:space:]]+[[:digit:]]+\)$/d; /^([[:space:]]+)?$/d`}
	for _, variable := range ember.Variables {
		value, expected := hidden[variable.Name]
		if variable.Hidden != expected || expected && (variable.Default != value || variable.Editable) {
			t.Fatalf("variable visibility/default changed: %+v", variable)
		}
		delete(hidden, variable.Name)
	}
	if len(hidden) != 0 {
		t.Fatal("missing hidden variables", hidden)
	}
	data, err := json.Marshal(ember)
	if err != nil {
		t.Fatal(err)
	}
	var restored model.Ember
	if err := json.Unmarshal(data, &restored); err != nil || restored.StartupShell != "bash" || !restored.Variables[5].Hidden {
		t.Fatal("native definition lost visibility or shell", err)
	}
}
