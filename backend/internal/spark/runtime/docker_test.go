package runtime

import (
	"slices"
	"testing"
)

func TestDockerConfigurationUsesOneLocalEndpoint(t *testing.T) {
	for _, test := range []struct {
		name    string
		host    string
		context string
		socket  string
	}{
		{name: "default", socket: "/var/run/docker.sock"},
		{name: "custom socket", host: "unix:///run/user/1000/docker.sock", socket: "/run/user/1000/docker.sock"},
		{name: "TCP", host: "tcp://localhost:2375"},
		{name: "SSH", host: "ssh://node"},
		{name: "relative socket", host: "unix://docker.sock"},
		{name: "empty socket", host: "unix://"},
		{name: "query", host: "unix:///run/docker.sock?other"},
		{name: "context", context: "remote"},
		{name: "context with socket", host: "unix:///run/docker.sock", context: "default"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("DOCKER_HOST", test.host)
			t.Setenv("DOCKER_CONTEXT", test.context)

			socket, err := dockerSocket()
			if test.socket == "" {
				if err == nil || ValidateDockerConfiguration() == nil {
					t.Fatal("unsupported transport accepted")
				}

				if _, err := dockerCommand(t.Context(), "inspect", "server"); err == nil {
					t.Fatal("CLI accepted an unsupported transport")
				}

				return
			}

			if err != nil || socket != test.socket {
				t.Fatal("wrong socket", socket, err)
			}

			command, err := dockerCommand(t.Context(), "inspect", "server")
			if err != nil {
				t.Fatal(err)
			}

			if !slices.Equal(command.Args, []string{"docker", "--host", "unix://" + socket, "inspect", "server"}) {
				t.Fatal("CLI endpoint differs from direct API endpoint", command.Args)
			}
		})
	}
}
