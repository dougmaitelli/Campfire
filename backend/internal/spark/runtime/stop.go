package runtime

import (
	"errors"
	"strings"

	"campfire/internal/shared/model"
)

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
