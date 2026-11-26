package model

import (
	"encoding/json"
	"testing"
)

func TestPrimaryHostPortRequiresMatchingProtocolAndUnambiguousHostPort(t *testing.T) {
	required := []RequiredPort{{Container: 25565, Protocol: NetworkTCP, Primary: true}}
	game := ServerPort{BindIP: "192.0.2.1", ContainerPort: 25565, HostPort: 30001, Protocol: NetworkTCP}
	other := game

	other.BindIP = "192.0.2.2"
	if got := PrimaryHostPort([]ServerPort{game, other}, required); got != 30001 {
		t.Fatal(got)
	}

	other.HostPort = 30002
	if got := PrimaryHostPort([]ServerPort{game, other}, required); got != 0 {
		t.Fatal("ambiguous mapping", got)
	}

	other.Protocol = NetworkUDP
	if got := PrimaryHostPort([]ServerPort{game, other}, required); got != 30001 {
		t.Fatal("other protocol affected selection", got)
	}

	if got := PrimaryHostPort([]ServerPort{other}, required); got != 0 {
		t.Fatal("wrong protocol matched", got)
	}

	game.HostPort = 0
	if got := PrimaryHostPort([]ServerPort{game}, required); got != 0 {
		t.Fatal("invalid host port", got)
	}
}

func TestPortableDefinitionPreservesPrimaryWithoutPlacement(t *testing.T) {
	c, e := deploymentSource()
	e.Schema = 2
	e.RequiredPorts[0].Primary = true

	encoded, err := json.Marshal(NewBackupDefinition(Server{ServerConfiguration: c}, e))
	if err != nil {
		t.Fatal(err)
	}

	var restored BackupDefinition
	if err = json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}

	port, ok := restored.Ember.PrimaryPort()
	if !ok || port.Container != 25565 || restored.Ember.Schema != 2 || len(restored.Configuration.Ports) != 0 || restored.Configuration.SparkID != "" {
		t.Fatal(restored)
	}
}
