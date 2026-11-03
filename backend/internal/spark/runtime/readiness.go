package runtime

import (
	"errors"
	"strings"

	"campfire/internal/shared/model"
)

func readinessMatches(ember model.RuntimeDefinition, logs string) bool {
	if ember.Readiness == nil {
		return strings.Contains(model.StripLogANSI(logs), ember.Ready)
	}

	matcher := ember.ReadinessMatcher()
	for _, line := range strings.Split(logs, "\n") {
		if matcher.Matches(strings.TrimSuffix(line, "\r")) {
			return true
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
