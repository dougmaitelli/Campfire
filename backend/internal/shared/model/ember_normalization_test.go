package model

import (
	"reflect"
	"testing"
)

func TestEmberNormalization(t *testing.T) {
	for _, test := range []struct {
		name    string
		schema  int
		ports   []RequiredPort
		primary int
	}{
		{"older with ports", 1, []RequiredPort{{Container: 25575, Protocol: NetworkTCP}, {Container: 25565, Protocol: NetworkTCP}}, 25575},
		{"older without ports", 1, nil, 0},
		{"current without primary", 2, []RequiredPort{{Container: 25565, Protocol: NetworkTCP}}, 0},
		{"current with primary", 2, []RequiredPort{{Container: 25575, Protocol: NetworkTCP}, {Container: 25565, Protocol: NetworkTCP, Primary: true}}, 25565},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := Ember{Schema: test.schema, RequiredPorts: test.ports}
			before := append([]RequiredPort(nil), source.RequiredPorts...)

			normalized := source.Normalized()
			if normalized.Schema != CurrentEmberSchema {
				t.Fatal("reader did not normalize schema")
			}

			port, ok := normalized.PrimaryPort()
			if port.Container != test.primary || ok != (test.primary != 0) {
				t.Fatal("unexpected primary", normalized)
			}

			if source.Schema != test.schema || !reflect.DeepEqual(source.RequiredPorts, before) {
				t.Fatal("normalization mutated its input")
			}

			if !reflect.DeepEqual(normalized.Normalized(), normalized) {
				t.Fatal("normalization is not idempotent")
			}
		})
	}

	for schema := 1; schema <= CurrentEmberSchema; schema++ {
		source, _ := emberReviewFixture(t)
		source.Schema = schema

		source.RequiredPorts = []RequiredPort{{Container: 25565, Protocol: NetworkTCP}}
		if err := source.Validate(); err != nil {
			t.Fatalf("supported schema %d rejected: %v", schema, err)
		}

		normalized := source.Normalized()
		if normalized.Schema != CurrentEmberSchema || normalized.RequiredPorts[0].Primary != (schema == 1) {
			t.Fatalf("schema %d did not normalize correctly: %+v", schema, normalized)
		}
	}

	for _, schema := range []int{0, -1, CurrentEmberSchema + 1} {
		source, _ := emberReviewFixture(t)

		source.Schema = schema
		if err := source.Validate(); err == nil {
			t.Fatal("unsupported version was not rejected by validation")
		}
	}
}
