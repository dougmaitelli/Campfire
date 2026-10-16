package runtime

import (
	"errors"
	"regexp"
	"strings"

	"campfire/internal/shared/model"
)

var terminalEscapes = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\))")

func readinessMatches(ember model.Ember, logs string) bool {
	if ember.Readiness == nil {
		return ember.Ready == "" || strings.Contains(logs, ember.Ready)
	}

	if ember.Readiness.StripANSI {
		logs = terminalEscapes.ReplaceAllString(logs, "")
	}

	for _, match := range ember.Readiness.Any {
		if !match.Regex && strings.Contains(logs, match.Value) {
			return true
		}

		if match.Regex {
			pattern, err := regexp.Compile(match.Value)
			if err != nil {
				continue
			}

			for _, line := range strings.Split(logs, "\n") {
				if pattern.MatchString(strings.TrimSuffix(line, "\r")) {
					return true
				}
			}
		}
	}

	return false
}

func stopCommand(d model.Deployment) (string, error) {
	command, err := renderBounded(d.Ember.Stop, environment(d), 1024)
	if err != nil {
		return "", err
	}

	if len(command) > 1024 || strings.ContainsAny(command, "\r\n\x00") {
		return "", errors.New("expanded stop command must be one line of at most 1024 bytes")
	}

	return command, nil
}
