package model

import (
	"encoding/hex"
	"errors"
	"time"
	"unicode/utf8"
)

// Crash bounds apply before persistence and transport, independently of readers.
const (
	CrashConsoleBytes = 16 << 10
	CrashReplayLimit  = 8
	CrashHistoryLimit = 100
	CrashRetention    = 30 * 24 * time.Hour
)

// CrashReason identifies a supported, evidence-backed cause.
type CrashReason string

// Supported crash reasons; unknown does not imply a specific cause.
const (
	CrashReasonUnknown     CrashReason = "unknown"
	CrashReasonOutOfMemory CrashReason = "out_of_memory"
)

// Valid rejects unsupported crash reasons.
func (r CrashReason) Valid() bool {
	return r == CrashReasonUnknown || r == CrashReasonOutOfMemory
}

// CrashRecord describes an abnormal runtime exit, not an installer failure or
// restart request. An exit code of 137 alone never proves an out-of-memory crash.
// Console is private evidence and must not appear in ordinary status responses.
type CrashRecord struct {
	ID                 string      `json:"id"`
	ContainerID        string      `json:"container_id"`
	ExitedAt           int64       `json:"exited_at"`
	ExitCode           int         `json:"exit_code"`
	Reason             CrashReason `json:"reason"`
	Console            string      `json:"console,omitempty"`
	ConsoleUnavailable bool        `json:"console_unavailable"`
	ConsoleTruncated   bool        `json:"console_truncated"`
}

// Validate checks bounded transport data without trusting the producer's clock.
func (c CrashRecord) Validate() error {
	for _, id := range []string{c.ID, c.ContainerID} {
		if len(id) != 64 {
			return errors.New("invalid crash identity")
		}

		if _, err := hex.DecodeString(id); err != nil {
			return errors.New("invalid crash identity")
		}
	}

	if !c.Reason.Valid() || c.ExitedAt <= 0 || c.ExitedAt > 253402300799 || c.ExitCode < 0 || c.ExitCode > 255 || len(c.Console) > CrashConsoleBytes || !utf8.ValidString(c.Console) {
		return errors.New("invalid crash record")
	}

	return nil
}

// Title is safe to expose without console permission; it contains no log text.
func (c CrashRecord) Title() string {
	if c.Reason == CrashReasonOutOfMemory {
		return "Server crashed — out of memory"
	}

	return "Server crashed"
}
