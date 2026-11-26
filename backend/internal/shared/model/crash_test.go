package model_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

func TestCrashReplayFitsFrameEvenWithEscapedConsoleEvidence(t *testing.T) {
	record := model.CrashRecord{ID: strings.Repeat("a", 64), ContainerID: strings.Repeat("b", 64), ExitedAt: time.Now().Unix(), ExitCode: 137, Reason: model.CrashReasonUnknown, Console: strings.Repeat("\x00", model.CrashConsoleBytes)}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}

	status := model.RuntimeStatus{}
	for range model.CrashReplayLimit {
		status.Crashes = append(status.Crashes, record)
	}

	data, err := json.Marshal(status)
	// Both WebSocket endpoints allow 2 MiB. Reserve the full serialized log
	// observation allowance and ample space for ordinary runtime metadata.
	if err != nil || len(data)+(512<<10)+(64<<10) >= 2<<20 {
		t.Fatal("crash replay exceeds transport budget", len(data), err)
	}

	if strings.Contains(record.Title(), "memory") {
		t.Fatal("137 alone became OOM proof")
	}
}
