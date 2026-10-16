package model

import (
	"fmt"
	"strconv"
	"strings"
)

// StopSignal resolves Egg signal notation to a Docker Linux signal name.
// An empty result identifies a console command or the image's default stop.
func StopSignal(stop string) (string, error) {
	if !strings.HasPrefix(stop, "^") {
		return "", nil
	}

	name := strings.ToUpper(strings.TrimPrefix(stop, "^"))
	if name == "C" {
		return "SIGINT", nil
	}

	name = strings.TrimPrefix(name, "SIG")
	switch name {
	case "ABRT", "ALRM", "BUS", "CHLD", "CLD", "CONT", "FPE", "HUP", "ILL", "INT",
		"IO", "IOT", "KILL", "PIPE", "POLL", "PROF", "PWR", "QUIT", "SEGV", "STKFLT",
		"STOP", "SYS", "TERM", "TRAP", "TSTP", "TTIN", "TTOU", "URG", "USR1", "USR2",
		"VTALRM", "WINCH", "XCPU", "XFSZ", "RTMIN", "RTMAX":
		return "SIG" + name, nil
	}

	for _, bound := range []struct {
		prefix string
		max    int
	}{{"RTMIN+", 15}, {"RTMAX-", 14}} {
		if suffix, ok := strings.CutPrefix(name, bound.prefix); ok {
			n, err := strconv.Atoi(suffix)
			if err == nil && n >= 1 && n <= bound.max && suffix == strconv.Itoa(n) {
				return "SIG" + name, nil
			}
		}
	}

	return "", fmt.Errorf("unsupported stop signal %q", stop)
}
