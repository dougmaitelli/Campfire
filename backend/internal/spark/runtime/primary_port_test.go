package runtime

import (
	"campfire/internal/shared/model"
	"testing"
)

func TestExplicitServerPortControlsRuntimeEnvironment(t *testing.T) {
	d := runtimeFixture()

	d.Configuration.Ports = []model.ServerPort{{BindIP: "0.0.0.0", ContainerPort: 25575, HostPort: 30002, Protocol: model.NetworkTCP}, {BindIP: "0.0.0.0", ContainerPort: 25565, HostPort: 30001, Protocol: model.NetworkTCP}}
	if got := environment(d)["SERVER_PORT"]; got != "25575" {
		t.Fatal("legacy snapshot changed", got)
	}

	d.Configuration.ServerPort = new(25565)
	if got := environment(d)["SERVER_PORT"]; got != "25565" {
		t.Fatal("explicit selection ignored", got)
	}

	d.Configuration.ServerPort = new(0)

	d.Configuration.Variables["SERVER_PORT"] = "12345"
	if got := environment(d)["SERVER_PORT"]; got != "" {
		t.Fatal("no-primary selection inferred a port", got)
	}

	d.Configuration.Ports = nil

	d.Configuration.ServerPort = new(25565)
	if got := environment(d)["SERVER_PORT"]; got != "25565" {
		t.Fatal("unpublished primary changed", got)
	}
}

func TestEmberPrimaryAgreesWithRuntimeAndPublishedMapping(t *testing.T) {
	e := model.Ember{ID: "game", Schema: 2, Name: "Game", Images: map[string]string{"Game": "alpine:3.22"}, Startup: "game --port {{SERVER_PORT}}", WorkDir: "/home/container", User: "1000:1000", RequiredPorts: []model.RequiredPort{{Container: 25575, Protocol: model.NetworkTCP}, {Container: 25565, Protocol: model.NetworkTCP, Primary: true}}}
	c := model.ServerConfiguration{EmberID: e.ID, SparkID: "node", Image: "alpine:3.22", CPULimit: 1, MemoryMB: 128, DiskMB: 128, Ports: []model.ServerPort{{BindIP: "0.0.0.0", ContainerPort: 25575, HostPort: 30002, Protocol: model.NetworkTCP}, {BindIP: "0.0.0.0", ContainerPort: 25565, HostPort: 30001, Protocol: model.NetworkTCP}}}

	d, err := model.NewDeployment("server", c, e)
	if err != nil {
		t.Fatal(err)
	}

	startup, err := model.Startup(d.Ember.Startup, environment(d))
	if err != nil || startup != "game --port ${SERVER_PORT}" || environment(d)["SERVER_PORT"] != "25565" || model.PrimaryHostPort(c.Ports, e.RequiredPorts) != 30001 {
		t.Fatal(startup, err)
	}
}
