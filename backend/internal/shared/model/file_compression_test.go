package model

import "testing"

func TestCompressionTerminalStates(t *testing.T) {
	for state, terminal := range map[CompressionState]bool{
		CompressionRunning:    false,
		CompressionCancelling: false,
		CompressionCompleted:  true,
		CompressionCancelled:  true,
		CompressionFailed:     true,
		"unknown":             false,
	} {
		if state.Terminal() != terminal {
			t.Fatalf("state %q: terminal=%v, expected %v", state, state.Terminal(), terminal)
		}
	}
}
