// Package runtime executes game server lifecycle operations through Docker.
package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
	"campfire/internal/shared/settings"
)

// Executor is the Docker command boundary, replaceable in tests.
type Executor interface {
	Run(context.Context, io.Reader, ...string) (string, error)
}

// Docker executes the installed Docker CLI without invoking a host shell.
type Docker struct{}

// TimestampedLogs captures original Docker log timestamps for saved installer output.
func (d Docker) TimestampedLogs(ctx context.Context, name string) (string, error) {
	return d.Run(ctx, nil, "logs", "--timestamps", name)
}

// ValidateDockerConfiguration rejects transports unsupported by the node runtime.
func ValidateDockerConfiguration() error {
	_, err := dockerSocket()
	return err
}

func dockerSocket() (string, error) {
	if settings.DockerContext.Value("") != "" {
		return "", fmt.Errorf("node runtime requires a local Docker Unix socket; unset %s and use %s=unix:///path/to/docker.sock if needed", settings.DockerContext.EnvironmentName, settings.DockerHost.EnvironmentName)
	}

	host := settings.DockerHost.Value("")
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	socket := strings.TrimPrefix(host, "unix://")
	if !strings.HasPrefix(host, "unix://") || !filepath.IsAbs(socket) || strings.ContainsAny(socket, "\x00?#") {
		return "", fmt.Errorf("node runtime requires %s to be a local Unix socket URL with an absolute path", settings.DockerHost.EnvironmentName)
	}

	return socket, nil
}

func dockerCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	socket, err := dockerSocket()
	if err != nil {
		return nil, err
	}

	// Pin CLI operations to the same endpoint as direct console and metrics calls,
	// regardless of the current context saved in Docker's configuration file.
	return exec.CommandContext(ctx, "docker", append([]string{"--host", "unix://" + socket}, args...)...), nil
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(value []byte) (int, error) {
	n := len(value)

	const limit = limits.LogCaptureBytes

	if n >= limit {
		b.Reset()

		value = value[n-limit:]
	} else if excess := b.Len() + n - limit; excess > 0 {
		b.Next(excess)
	}

	_, _ = b.Buffer.Write(value)

	return n, nil
}

// Run captures bounded output; errors never disclose environment values or scripts.
func (Docker) Run(ctx context.Context, input io.Reader, args ...string) (string, error) {
	command, err := dockerCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	command.Stdin = input
	output := &boundedOutput{}

	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		return output.String(), &runtimeFailure{code: model.RuntimeDockerFailed, cause: err, message: "Docker could not complete the operation. Check the Docker and server logs on the Spark."}
	}

	return output.String(), nil
}
