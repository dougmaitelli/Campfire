package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExplicitPrimaryPortIsIndependentOfOrdering(t *testing.T) {
	c, e := deploymentSource()
	e.Schema = 2
	e.RequiredPorts = []RequiredPort{{Container: 25575, Protocol: NetworkTCP}, {Container: 25565, Protocol: NetworkTCP, Primary: true}}
	c.Ports = []ServerPort{{BindIP: "0.0.0.0", ContainerPort: 25575, HostPort: 30002, Protocol: NetworkTCP}, {BindIP: "0.0.0.0", ContainerPort: 25565, HostPort: 30001, Protocol: NetworkTCP}}

	first, err := NewDeployment("server", c, e)
	if err != nil {
		t.Fatal(err)
	}

	if first.Configuration.ServerPort == nil || *first.Configuration.ServerPort != 25565 || PrimaryHostPort(c.Ports, e.RequiredPorts) != 30001 {
		t.Fatal(first)
	}

	e.RequiredPorts[0], e.RequiredPorts[1] = e.RequiredPorts[1], e.RequiredPorts[0]

	second, err := NewDeployment("server", c, e)
	if err != nil || DeploymentHash(first) != DeploymentHash(second) {
		t.Fatal("requirement order changed execution", err)
	}

	c.Ports[0], c.Ports[1] = c.Ports[1], c.Ports[0]

	third, err := NewDeployment("server", c, e)
	if err != nil || *third.Configuration.ServerPort != 25565 || PrimaryHostPort(c.Ports, e.RequiredPorts) != 30001 {
		t.Fatal(third, err)
	}
	// A primary container port can exist without a published mapping.
	c.Ports = nil

	private, err := NewDeployment("server", c, e)
	if err != nil || *private.Configuration.ServerPort != 25565 || PrimaryHostPort(nil, e.RequiredPorts) != 0 {
		t.Fatal(private, err)
	}
}

func TestNoPrimaryNeverInfersFromRequiredPortsOrMappings(t *testing.T) {
	c, e := deploymentSource()
	e.Schema = 2

	c.Ports = []ServerPort{{BindIP: "0.0.0.0", ContainerPort: 25565, HostPort: 30001, Protocol: NetworkTCP}}
	for _, ports := range [][]ServerPort{c.Ports, nil} {
		c.Ports = ports

		d, err := NewDeployment("server", c, e)
		if err != nil || d.Configuration.ServerPort == nil || *d.Configuration.ServerPort != 0 || PrimaryHostPort(ports, e.RequiredPorts) != 0 {
			t.Fatal(d, err)
		}
	}

	e.RequiredPorts = nil

	d, err := NewDeployment("server", c, e)
	if err != nil || *d.Configuration.ServerPort != 0 {
		t.Fatal(d, err)
	}
}

func TestLegacyEmberDeploymentRemainsUnchangedUntilAdopted(t *testing.T) {
	c, e := deploymentSource()
	e.Variables[0].Editable = true

	legacy, err := NewDeployment("server", c, e)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(legacy)
	if err != nil || legacy.Configuration.ServerPort != nil || strings.Contains(string(encoded), "server_port") || legacy.Configuration.Variables["SERVER_PORT"] != "25565" {
		t.Fatal(string(encoded), err)
	}

	e.Schema = 2
	e.RequiredPorts[0].Primary = true

	adopted, err := NewDeployment("server", c, e)
	if err != nil || adopted.Configuration.ServerPort == nil || *adopted.Configuration.ServerPort != 25565 || DeploymentHash(legacy) == DeploymentHash(adopted) {
		t.Fatal(adopted, err)
	}
	// Adoption is explicit and produces a configuration-sync/restart difference.
	server := Server{ID: "server", ServerConfiguration: c, EmberHash: DeploymentHash(legacy)}
	server.SetEmberStatus(e)

	if server.EmberReviewRequired || !server.DeploymentSyncRequired {
		t.Fatal(server)
	}
}

func TestPrimaryPortDefinitionValidation(t *testing.T) {
	_, e := deploymentSource()

	e.RequiredPorts[0].Primary = true
	if e.Validate() == nil {
		t.Fatal("legacy schema accepted a new designation")
	}

	e.Schema = 2
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}

	e.RequiredPorts = append(e.RequiredPorts, RequiredPort{Container: 25575, Protocol: NetworkTCP, Primary: true})
	if e.Validate() == nil {
		t.Fatal("multiple primary ports accepted")
	}

	e.RequiredPorts[1].Primary = false

	e.RequiredPorts[1].Container = 25565
	if err := e.Validate(); err != nil {
		t.Fatal("duplicate non-primary requirements changed existing validation", err)
	}

	e.Schema = 3
	if e.Validate() == nil {
		t.Fatal("unknown schema accepted")
	}
}
