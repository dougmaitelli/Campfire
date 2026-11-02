package runtime

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
)

var terminalEscapes = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\))")

func (m *Manager) containerReady(ctx context.Context, d model.Deployment, since string) bool {
	if docker, ok := m.Docker.(interface {
		Ready(context.Context, string, string, model.RuntimeDefinition) (bool, error)
	}); ok {
		ready, err := docker.Ready(ctx, container(d.ServerID), since, d.Ember)
		return err == nil && ready
	}

	logs, err := m.Docker.Run(ctx, nil, "logs", "--since", since, container(d.ServerID))

	return err == nil && readinessMatches(d.Ember, logs)
}

// Ready searches retained output from this container run without keeping the
// entire log in memory. An old startup message can be far beyond the log tail.
func (Docker) Ready(ctx context.Context, name, since string, ember model.RuntimeDefinition) (bool, error) {
	command, err := dockerCommand(ctx, "logs", "--since", since, name)
	if err != nil {
		return false, err
	}

	output := &readinessOutput{ember: ember}

	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		return false, err
	}

	return output.ready, nil
}

type readinessOutput struct {
	ember  model.RuntimeDefinition
	buffer boundedOutput
	ready  bool
}

func (o *readinessOutput) Write(data []byte) (int, error) {
	n := len(data)
	for len(data) > 0 && !o.ready {
		size := min(len(data), limits.LogCaptureBytes/2)
		_, _ = o.buffer.Write(data[:size])
		o.ready = readinessMatches(o.ember, o.buffer.String())
		data = data[size:]
	}

	return n, nil
}

func readinessMatches(ember model.RuntimeDefinition, logs string) bool {
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
