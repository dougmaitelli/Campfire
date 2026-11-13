package integrations

import (
	"fmt"
	"reflect"
	"testing"

	"campfire/internal/shared/model"
)

func TestServerCardSelectsOnlyRequestedFreshValues(t *testing.T) {
	server := model.Server{ServerConfiguration: model.ServerConfiguration{Name: "Survival", Variables: map[string]string{"TOKEN": "private"}}}
	settings := CardSettings{Enabled: true, Fields: []string{"players", "capacity", "field.world", "field.invite_code", "field.pvp", "field.round", "field.missing"}}
	game := &model.GameState{Players: new(0), Capacity: new(20), Fields: map[string]any{"world": "Meadows", "invite_code": "ABCD", "pvp": false, "round": float64(0), "private": "hidden"}}
	game.FieldLabels = model.GameFieldLabels{"world": "World name", "invite_code": "Join code"}
	card := BuildServerCard(server, model.RuntimeStateRunning, game, settings, game.FieldLabels)

	want := []CardField{
		{ID: "players", Name: "Players", Value: "0 / 20", Available: true},
		{ID: "field.world", Name: "World name", Value: "Meadows", Available: true},
		{ID: "field.invite_code", Name: "Join code", Value: "ABCD", Available: true},
		{ID: "field.pvp", Name: "pvp", Value: "No", Available: true},
		{ID: "field.round", Name: "round", Value: "0", Available: true},
		{ID: "field.missing", Name: "missing"},
	}
	if card.Name != "Survival" || !reflect.DeepEqual(card.Fields, want) {
		t.Fatal(card)
	}

	for _, state := range []model.RuntimeState{model.RuntimeStateStopped, model.RuntimeStateUnknown, model.RuntimeStateRestarting} {
		card = BuildServerCard(server, state, game, settings, game.FieldLabels)
		for _, field := range card.Fields {
			if field.Available || field.Value != "" {
				t.Fatal("non-running card exposed game state", state, field)
			}
		}
	}
}

func TestServerCardAddressResolution(t *testing.T) {
	for _, test := range []struct {
		name  string
		host  string
		ports []model.ServerPort
		want  []string
	}{
		{"public address", "games.example.test", []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25565, Protocol: model.NetworkTCP}}, []string{"games.example.test:25565 / TCP"}},
		{"IPv6", "2001:db8::1", []model.ServerPort{{BindIP: "::", HostPort: 7777, Protocol: model.NetworkUDP}}, []string{"[2001:db8::1]:7777 / UDP"}},
		{"specific bind", "", []model.ServerPort{{BindIP: "192.0.2.10", HostPort: 25565, Protocol: model.NetworkTCP}}, []string{"192.0.2.10:25565 / TCP"}},
		{"unavailable wildcard", "", []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 25565, Protocol: model.NetworkTCP}, {BindIP: "::", HostPort: 7777, Protocol: model.NetworkUDP}}, []string{}},
		{"address without ports", "games.example.test", nil, []string{"games.example.test"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := model.Server{GameAddress: test.host, ServerConfiguration: model.ServerConfiguration{Ports: test.ports}}
			if got := BuildServerCard(server, model.RuntimeStateStopped, nil, CardSettings{}, nil).Addresses; !reflect.DeepEqual(got, test.want) {
				t.Fatal(got, test.want)
			}
		})
	}
}

func TestServerCardSettingsRejectUnknownDuplicateAndExcessiveFields(t *testing.T) {
	for _, fields := range [][]string{{"variables.token"}, {"players", "players"}, {"field."}, {"field.Invalid"}} {
		if (CardSettings{Fields: fields}).Validate() == nil {
			t.Fatal("invalid selection accepted", fields)
		}
	}

	fields := make([]string, 33)
	for index := range fields {
		fields[index] = fmt.Sprintf("field.value%d", index)
	}

	if (CardSettings{Fields: fields}).Validate() == nil {
		t.Fatal("excessive selection accepted")
	}
}
