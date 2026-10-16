package model

import "testing"

func TestStopSignal(t *testing.T) {
	for input, want := range map[string]string{
		"": "", "stop": "", "quit {{REASON}}": "", "^C": "SIGINT",
		"^SIGINT": "SIGINT", "^SIGTERM": "SIGTERM", "^SIGABRT": "SIGABRT",
		"^SIGKILL": "SIGKILL", "^SIGQUIT": "SIGQUIT", "^SIGUSR2": "SIGUSR2",
		"^hup": "SIGHUP", "^sigterm": "SIGTERM", "^SIGRTMIN": "SIGRTMIN",
		"^RTMIN+15": "SIGRTMIN+15", "^SIGRTMAX-14": "SIGRTMAX-14",
	} {
		t.Run(input, func(t *testing.T) {
			got, err := StopSignal(input)
			if err != nil || got != want {
				t.Fatalf("StopSignal(%q) = %q, %v; want %q", input, got, err, want)
			}
		})
	}

	for _, input := range []string{"^", "^INVALID", "^0", "^SIGTERM\n", "^SIGTERM --time 0", "^SIGRTMIN+16", "^SIGRTMAX-15", "^RTMIN+01", "^RTMIN+-1"} {
		if _, err := StopSignal(input); err == nil {
			t.Errorf("accepted invalid signal %q", input)
		}
	}
}
