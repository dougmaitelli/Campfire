package runtime

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"campfire/internal/shared/settings"
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
			t.Setenv(settings.DockerHost.EnvironmentName, test.host)
			t.Setenv(settings.DockerContext.EnvironmentName, test.context)

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

func TestGameOwnershipUsesDockerUIDAndGIDMaps(t *testing.T) {
	uids, err := parseIDMap([]byte("         0 1001 1\n1 120000 1000\n1001 900000 12\n"))
	if err != nil {
		t.Fatal(err)
	}

	gids, err := parseIDMap([]byte("0 1002 1\n1 300000 65536"))
	if err != nil {
		t.Fatal(err)
	}

	n := &dockerNamespace{uids: uids, gids: gids}
	for _, tc := range []struct {
		namespace *dockerNamespace
		user      string
		uid, gid  int
	}{
		{nil, "1000:123", 1000, 123},
		{n, "1000", 120999, 300999},
		{n, "1001:123", 900000, 300122},
	} {
		uid, gid, err := tc.namespace.gameOwner(tc.user)
		if err != nil || uid != tc.uid || gid != tc.gid {
			t.Fatalf("gameOwner(%s) = %d:%d, %v", tc.user, uid, gid, err)
		}
	}

	for _, user := range []string{"0", "1013", "1000:65537", "1000:-1", "1:2:3", "4294967295"} {
		if _, _, err := n.gameOwner(user); err == nil {
			t.Errorf("accepted %s", user)
		}
	}

	for _, mapping := range []string{"0 0 1", "0 1000 0", "1 4294967294 8", "0 abc 1", "0 1"} {
		if _, err := parseIDMap([]byte(mapping)); err == nil {
			t.Errorf("accepted map %q", mapping)
		}
	}
}

type infoDocker string

func (d infoDocker) Run(context.Context, io.Reader, ...string) (string, error) {
	return string(d), nil
}

func TestDockerNamespacePreflight(t *testing.T) {
	m := &Manager{Docker: infoDocker(`{"SecurityOptions":["name=seccomp,profile=builtin"]}`)}
	if err := m.ConfigureDocker(t.Context()); err != nil || m.namespace != nil {
		t.Fatal("rootful preflight", err)
	}

	m.Docker = infoDocker(`{"SecurityOptions":["name=rootless,version=1"],"CgroupVersion":"2","CgroupDriver":"systemd","MemoryLimit":true,"SwapLimit":true,"PidsLimit":true}`)
	if err := m.ConfigureDocker(t.Context()); err == nil || !strings.Contains(err.Error(), "delegated CPU") {
		t.Fatal("accepted rootless daemon without CPU delegation", err)
	}
}
