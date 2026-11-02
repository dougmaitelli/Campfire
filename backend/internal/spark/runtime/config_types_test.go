package runtime

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"campfire/internal/shared/model"

	"gopkg.in/yaml.v3"
)

func TestStructuredConfigUsesExistingTypesAndInfersMissingScalars(t *testing.T) {
	for _, parser := range []model.ConfigParser{model.ConfigParserJSON, model.ConfigParserYAML} {
		t.Run(string(parser), func(t *testing.T) {
			source := []byte(`{"text":"before","number":1,"flag":true,"null":null,"items":[{"value":"old"},{"value":1},{"value":false},{"value":null},{}]}`)
			config := model.ConfigFile{Parser: parser, Values: map[string]string{
				"text": "123", "number": "9007199254740995", "flag": "false", "null": "true",
				"newNumber": "12.5", "newBool": "true", "newText": "001", "newNull": "null",
				"newObject": `{"nested":1}`, "items.*.value": "42",
			}}

			updated, err := structuredConfig(source, config, config.Values)
			if err != nil {
				t.Fatal(err)
			}

			if parser == model.ConfigParserYAML {
				var document any

				if err := yaml.Unmarshal(updated, &document); err != nil {
					t.Fatal(err)
				}

				updated, err = json.Marshal(document)
				if err != nil {
					t.Fatal(err)
				}
			}

			var actual map[string]any

			decoder := json.NewDecoder(bytes.NewReader(updated))
			decoder.UseNumber()

			if err := decoder.Decode(&actual); err != nil {
				t.Fatal(err)
			}

			expected := map[string]any{
				"text": "123", "number": json.Number("9007199254740995"), "flag": false, "null": "true",
				"newNumber": json.Number("12.5"), "newBool": true, "newText": "001", "newNull": "null",
				"newObject": `{"nested":1}`, "items": []any{
					map[string]any{"value": "42"}, map[string]any{"value": json.Number("42")},
					map[string]any{"value": "42"}, map[string]any{"value": "42"}, map[string]any{"value": json.Number("42")},
				},
			}

			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("unexpected replacement types:\n%s\nwant: %#v", updated, expected)
			}
		})
	}
}

func TestStructuredConfigInvalidTypedValuesFallBackToStrings(t *testing.T) {
	for _, value := range []string{"001", "+5", "NaN", " 12 ", "12x", "1.20.1", "null", "[]", `{"nested":1}`} {
		t.Run(value, func(t *testing.T) {
			config := model.ConfigFile{Parser: model.ConfigParserJSON, Values: map[string]string{"number": value, "flag": value, "missing": value}}

			updated, err := structuredConfig([]byte(`{"number":1,"flag":false}`), config, config.Values)
			if err != nil {
				t.Fatal(err)
			}

			var actual map[string]any

			if err := json.Unmarshal(updated, &actual); err != nil {
				t.Fatal(err)
			}

			for _, key := range []string{"number", "flag", "missing"} {
				if actual[key] != value {
					t.Fatalf("%s converted %q to %#v", key, value, actual[key])
				}
			}
		})
	}
}

func TestStructuredConfigPreservesBooleanAndArraySlotSemantics(t *testing.T) {
	config := model.ConfigFile{Parser: model.ConfigParserJSON, Values: map[string]string{"existing": "1", "new": "1", "items.0": "12", "items.1": "12"}}

	updated, err := structuredConfig([]byte(`{"existing":false,"items":[null]}`), config, config.Values)
	if err != nil {
		t.Fatal(err)
	}

	var actual map[string]any

	if err := json.Unmarshal(updated, &actual); err != nil {
		t.Fatal(err)
	}

	if actual["existing"] != true || actual["new"] != float64(1) || !reflect.DeepEqual(actual["items"], []any{"12", float64(12)}) {
		t.Fatalf("type inference ignored existing entries: %s", updated)
	}
}

func TestStructuredConfigInfersExpandedArraySlots(t *testing.T) {
	for _, parser := range []model.ConfigParser{model.ConfigParserJSON, model.ConfigParserYAML} {
		t.Run(string(parser), func(t *testing.T) {
			config := model.ConfigFile{Parser: parser, Values: map[string]string{"items.10": "10", "items.2": "2"}}

			updated, err := structuredConfig([]byte(`{"items":[null]}`), config, config.Values)
			if err != nil {
				t.Fatal(err)
			}

			var actual struct {
				Items []any `json:"items" yaml:"items"`
			}

			if parser == model.ConfigParserYAML {
				err = yaml.Unmarshal(updated, &actual)
			} else {
				err = json.Unmarshal(updated, &actual)
			}

			if err != nil {
				t.Fatal(err)
			}

			if len(actual.Items) != 11 || actual.Items[0] != nil || actual.Items[1] != nil {
				t.Fatalf("array gaps must remain null: %s", updated)
			}

			for _, index := range []int{2, 10} {
				if actual.Items[index] != float64(index) && actual.Items[index] != index {
					t.Fatalf("missing array entry %d was not inferred as numeric: %s", index, updated)
				}
			}
		})
	}
}
