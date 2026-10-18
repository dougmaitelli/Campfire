package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/shared/protocol"
)

func TestVerifySparkUpdateCompatibility(t *testing.T) {
	for _, test := range []struct {
		name  string
		info  string
		valid bool
	}{
		{"compatible", `{"version":"v1.1.0","minimum_protocol":1,"protocol":7}`, true},
		{"wrong version", `{"version":"v1.2.0","minimum_protocol":1,"protocol":7}`, false},
		{"future protocol", `{"version":"v1.1.0","minimum_protocol":8,"protocol":8}`, false},
		{"old protocol", `{"version":"v1.1.0","minimum_protocol":1,"protocol":6}`, false},
		{"invalid response", `invalid`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			executable := filepath.Join(t.TempDir(), "spark")
			if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s' '"+test.info+"'\n"), 0755); err != nil {
				t.Fatal(err)
			}

			if err := verifySparkUpdate(t.Context(), executable, "v1.1.0", 7); (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}

func TestSelfUpdateRequiresCapabilityAndAcknowledgesBeforeRestart(t *testing.T) {
	calls := 0
	store := &ConfigurationStore{Update: func(_ context.Context, version string) (string, error) {
		calls++

		if version != "v1.1.0" {
			t.Fatal(version)
		}

		return "/usr/local/bin/spark", nil
	}}
	command := protocol.Command{ID: "update", Name: protocol.UpdateSpark, SparkVersion: "v1.1.0", Deadline: time.Now().Add(time.Minute)}

	denied := store.executeNegotiated(t.Context(), "node", command, nil)
	if denied.Error == nil || calls != 0 || store.RestartExecutable != "" {
		t.Fatal("unnegotiated update executed", denied)
	}

	accepted := store.executeNegotiated(t.Context(), "node", command, []string{protocol.CapabilitySelfUpdate})
	if accepted.Error != nil || accepted.SparkVersion != "v1.1.0" || calls != 1 || store.RestartExecutable == "" {
		t.Fatal("missing update acknowledgement", accepted)
	}
}
