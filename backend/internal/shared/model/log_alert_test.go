package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLogAlertIdentityUsesMatchingBehavior(t *testing.T) {
	rule := LogAlert{Title: "Problem", Message: "Fix it", Severity: LogError, Any: []LogMatch{{Value: "Fatal", IgnoreCase: true}, {Value: "^error [0-9]+$", Regex: true, IgnoreCase: true}}}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}

	key := rule.Key()
	rule.Title, rule.Message, rule.Severity = "New title", "New guidance", LogWarning

	rule.Any = []LogMatch{rule.Any[1], {Value: "FATAL", IgnoreCase: true}, {Value: "fatal", IgnoreCase: true}}
	if rule.Key() != key {
		t.Fatal("presentation edits or reordered equivalent matches changed identity")
	}

	rule.Any[0].Value = "^different$"
	if rule.Key() == key {
		t.Fatal("changing matching conditions retained the old identity")
	}
}

func TestLogMatchesHaveIndependentCaseBehaviorAndAlwaysIgnoreColor(t *testing.T) {
	matches := []LogMatch{{Value: "Exact"}, {Value: "folded", IgnoreCase: true}, {Value: "^Regex [0-9]+$", Regex: true, IgnoreCase: true}, {Value: "^Strict [0-9]+$", Regex: true}}

	matcher := CompileLogMatcher(matches)
	for _, test := range []struct {
		line    string
		matches bool
	}{
		{"\x1b[31mExact\x1b[0m", true}, {"exact", false}, {"FOLDED", true},
		{"\x1b[32mREGEX 123\x1b[0m", true}, {"Strict 123", true}, {"STRICT 123", false},
	} {
		if matcher.Matches(test.line) != test.matches {
			t.Fatalf("incorrect case or color handling for %q", test.line)
		}
	}

	rule := LogAlert{Any: matches}
	key := rule.Key()

	rule.Any[0].IgnoreCase = true
	if rule.Key() == key {
		t.Fatal("changing one condition's case behavior did not change identity")
	}

	for _, definition := range []RuntimeDefinition{
		{Ready: "Ready"}, {Readiness: &Readiness{Any: []LogMatch{{Value: "^Ready$", Regex: true}}}},
	} {
		if !definition.ReadinessMatcher().Matches("\x1b[31mReady\x1b[0m") {
			t.Fatal("readiness did not ignore console colors")
		}
	}
}

func TestLogAlertDecodesEarlierRuleWideCaseFlag(t *testing.T) {
	var rule LogAlert
	if err := json.Unmarshal([]byte(`{"title":"Problem","message":"Fix","severity":"error","ignore_case":true,"strip_ansi":true,"any":[{"value":"Fatal"},{"value":"^Error$","regex":true}]}`), &rule); err != nil {
		t.Fatal(err)
	}

	if !CompileLogMatcher(rule.Any).Matches("FATAL") || !CompileLogMatcher(rule.Any).Matches("ERROR") {
		t.Fatal("earlier rule lost case-insensitive matching")
	}

	if rule.Key() != "76019351881331f2c64407ae589878cf19bb04201476c8776ca89bed304efff8" {
		t.Fatal("moving the case flag changed an existing rule's identity")
	}

	encoded, err := json.Marshal(rule)
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}

	if document["ignore_case"] != nil || document["strip_ansi"] != nil {
		t.Fatal("obsolete rule flags were serialized", string(encoded))
	}
}

func TestLogObservationsBoundEscapedHistoryAndUnicodeEvidence(t *testing.T) {
	evidence := LogEvidence("\x1b[31m" + strings.Repeat("界", 600) + "\x1b[0m\n\x00")
	if len(evidence) > 1024 || !utf8.ValidString(evidence) || strings.ContainsAny(evidence, "\x1b\x00\n") {
		t.Fatal("console evidence retained controls or cut a UTF-8 character")
	}

	items := make([]DetectedLogAlert, 0, LogAlertHistoryLimit)
	for index := range LogAlertHistoryLimit {
		items = append(items, DetectedLogAlert{ID: fmt.Sprintf("%064x", index+1), RuleKey: strings.Repeat("a", 64), ExecutionID: strings.Repeat("b", 64), Phase: LogRuntime, Title: "Problem", Message: strings.Repeat("<", 2048), Severity: LogError, DetectedAt: 1, Line: evidence})
	}

	retained := RetainLogAlerts(items)
	if len(retained) == 0 || len(retained) == len(items) || retained[len(retained)-1].ID != items[len(items)-1].ID {
		t.Fatal("escaped JSON history did not retain its newest bounded observations")
	}

	observation := LogObservation{Executions: map[LogPhase]string{LogRuntime: strings.Repeat("b", 64)}, Alerts: retained}
	if err := observation.Validate(); err != nil {
		t.Fatal("retained history does not fit the transport contract", err)
	}
}

func TestLogAlertValidationAndDeploymentProjection(t *testing.T) {
	rule := LogAlert{Title: "EULA", Message: "Review eula.txt", Severity: LogError, Any: []LogMatch{{Value: "agree to the EULA"}}, Action: &LogAlertAction{Type: LogOpenFile, Path: "eula.txt"}}
	for _, invalid := range []LogAlert{
		{Title: "", Message: "Fix", Severity: LogError, Any: rule.Any},
		{Title: "Bad regex", Message: "Fix", Severity: LogError, Any: []LogMatch{{Value: "(", Regex: true}}},
		{Title: "Bad severity", Message: "Fix", Severity: "critical", Any: rule.Any},
		{Title: "Bad phase", Message: "Fix", Severity: LogError, Phase: "stopped", Any: rule.Any},
		{Title: "No matches", Message: "Fix", Severity: LogError},
		{Title: "Unsafe file", Message: "Fix", Severity: LogError, Any: rule.Any, Action: &LogAlertAction{Type: LogOpenFile, Path: "../outside"}},
		{Title: "Command", Message: "Fix", Severity: LogError, Any: rule.Any, Action: &LogAlertAction{Type: "shell"}},
	} {
		if invalid.Validate() == nil {
			t.Fatalf("accepted invalid rule: %+v", invalid)
		}
	}

	runtime := RuntimeDefinition{WorkDir: "/home/container", User: "1000:1000", LogAlerts: []LogAlert{rule, rule}}
	if err := runtime.validate(nil); err == nil || !strings.Contains(err.Error(), "unique") {
		t.Fatal("duplicate matching behavior accepted", err)
	}

	runtime.LogAlerts = []LogAlert{rule}
	if err := runtime.validate(nil); err != nil {
		t.Fatal(err)
	}
}
