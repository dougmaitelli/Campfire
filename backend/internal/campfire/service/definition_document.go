package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// definitionDocument normalizes bounded JSON/YAML without accepting ambiguous keys or aliases.
func definitionDocument(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > maxEmberBytes {
		return nil, errors.New("definitions must contain 1 byte to 1 MiB")
	}

	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))

	var value any

	var err error

	if len(data) > 0 && (data[0] == '{' || data[0] == '[') {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()

		remaining := 100000

		value, err = jsonDocumentValue(decoder, 0, &remaining)
		if err != nil {
			return nil, err
		}

		if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
			return nil, errors.New("use exactly one JSON definition")
		}
	} else {
		decoder := yaml.NewDecoder(bytes.NewReader(data))

		var node yaml.Node
		if err := decoder.Decode(&node); err != nil {
			return nil, errors.New("invalid JSON or YAML definition")
		}

		var extra yaml.Node
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return nil, errors.New("use exactly one YAML document")
		}

		remaining := 100000

		value, err = yamlDocumentValue(&node, 0, &remaining)
		if err != nil {
			return nil, err
		}
	}

	if _, ok := value.(map[string]any); !ok {
		return nil, errors.New("definition must be an object")
	}

	return json.Marshal(value)
}

func jsonDocumentValue(d *json.Decoder, depth int, remaining *int) (any, error) {
	*remaining--
	if depth > 64 || *remaining < 0 {
		return nil, errors.New("definition structure exceeds limits")
	}

	token, err := d.Token()
	if err != nil {
		return nil, errors.New("invalid JSON definition")
	}

	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}

	switch delim {
	case '{':
		object := map[string]any{}

		for d.More() {
			raw, err := d.Token()
			if err != nil {
				return nil, errors.New("invalid JSON object")
			}

			key, ok := raw.(string)
			if !ok {
				return nil, errors.New("invalid JSON key")
			}

			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate definition key %q", key)
			}

			child, err := jsonDocumentValue(d, depth+1, remaining)
			if err != nil {
				return nil, err
			}

			object[key] = child
		}

		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return nil, errors.New("invalid JSON object")
		}

		return object, nil
	case '[':
		array := []any{}

		for d.More() {
			child, err := jsonDocumentValue(d, depth+1, remaining)
			if err != nil {
				return nil, err
			}

			array = append(array, child)
		}

		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return nil, errors.New("invalid JSON array")
		}

		return array, nil
	default:
		return nil, errors.New("invalid JSON definition")
	}
}

func yamlDocumentValue(n *yaml.Node, depth int, remaining *int) (any, error) {
	*remaining--
	if depth > 64 || *remaining < 0 {
		return nil, errors.New("definition structure exceeds limits")
	}

	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) != 1 {
			return nil, errors.New("empty YAML document")
		}

		return yamlDocumentValue(n.Content[0], depth+1, remaining)
	case yaml.MappingNode:
		if n.Tag != "!!map" {
			return nil, errors.New("custom YAML tags are unsupported")
		}

		object := map[string]any{}

		for i := 0; i < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "<<" {
				return nil, errors.New("YAML keys must be strings; merge keys are unsupported")
			}

			if _, ok := object[key.Value]; ok {
				return nil, fmt.Errorf("duplicate definition key %q", key.Value)
			}

			child, err := yamlDocumentValue(n.Content[i+1], depth+1, remaining)
			if err != nil {
				return nil, err
			}

			object[key.Value] = child
		}

		return object, nil
	case yaml.SequenceNode:
		if n.Tag != "!!seq" {
			return nil, errors.New("custom YAML tags are unsupported")
		}

		values := []any{}

		for _, child := range n.Content {
			value, err := yamlDocumentValue(child, depth+1, remaining)
			if err != nil {
				return nil, err
			}

			values = append(values, value)
		}

		return values, nil
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!str", "!!timestamp":
			return n.Value, nil
		case "!!null":
			return nil, nil
		case "!!bool":
			value, err := strconv.ParseBool(strings.ToLower(n.Value))
			return value, err
		case "!!int", "!!float":
			var value any
			if err := n.Decode(&value); err != nil {
				return nil, errors.New("invalid YAML number")
			}

			if number, ok := value.(float64); ok && (math.IsNaN(number) || math.IsInf(number, 0)) {
				return nil, errors.New("YAML numbers must be finite")
			}

			return value, nil
		default:
			return nil, errors.New("unsupported YAML scalar tag")
		}
	default:
		return nil, errors.New("YAML aliases are unsupported; expand them before importing")
	}
}

func strictDefinition(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	return nil
}
