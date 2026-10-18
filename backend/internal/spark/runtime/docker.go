// Package runtime executes game server lifecycle operations through Docker.
package runtime

import (
	"bytes"
	"context"
	"io"
	"os/exec"

	"campfire/internal/shared/model"
)

// Executor is the Docker command boundary, replaceable in tests.
type Executor interface {
	Run(context.Context, io.Reader, ...string) (string, error)
}

// Docker executes the installed Docker CLI without invoking a host shell.
type Docker struct{}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(value []byte) (int, error) {
	n := len(value)
	if available := 65536 - b.Len(); available > 0 {
		_, _ = b.Buffer.Write(value[:min(available, n)])
	}

	return n, nil
}

// Run captures bounded output; errors never disclose environment values or scripts.
func (Docker) Run(ctx context.Context, input io.Reader, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "docker", args...)
	command.Stdin = input
	output := &boundedOutput{}

	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		return output.String(), &runtimeFailure{code: model.RuntimeDockerFailed, cause: err, message: "Docker could not complete the operation. Check the Docker and server logs on the Spark."}
	}

	return output.String(), nil
}
