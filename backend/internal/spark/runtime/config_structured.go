package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"

	"campfire/internal/shared/model"

	"gopkg.in/yaml.v3"
)

func structuredConfig(old []byte, config model.ConfigFile, values map[string]string) ([]byte, error) {
	if config.Parser == model.ConfigParserYAML {
		decoder := yaml.NewDecoder(bytes.NewReader(old))

		var document yaml.Node
		if err := decoder.Decode(&document); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}

		var extra yaml.Node
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return nil, errors.New("configuration must contain one YAML document")
		}

		if err := validateConfigYAML(&document, 0); err != nil {
			return nil, err
		}

		var decoded any
		if len(document.Content) > 0 {
			if err := document.Decode(&decoded); err != nil {
				return nil, err
			}
		}

		if decoded == nil {
			decoded = map[string]any{}
		}

		var err error

		old, err = json.Marshal(decoded)
		if err != nil {
			return nil, errors.New("YAML configuration requires string mapping keys and finite scalar values")
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(old))
	decoder.UseNumber()

	var object any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}

	if _, ok := object.(map[string]any); !ok {
		return nil, errors.New("configuration must be an object or mapping")
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected trailing configuration data")
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	budget := 100000

	for _, key := range keys {
		if err := model.ConfigKey(config.Parser, key); err != nil {
			return nil, err
		}

		var value any = values[key]

		literal := false

		for _, name := range config.Strings {
			if name == key {
				literal = true
				break
			}
		}

		if !literal {
			parsed := json.NewDecoder(strings.NewReader(values[key]))
			parsed.UseNumber()

			var scalar any
			if parsed.Decode(&scalar) == nil && errors.Is(parsed.Decode(&extra), io.EOF) {
				value = scalar
			}
		}

		var err error

		object, err = setConfigValue(object, strings.Split(key, "."), value, &budget)
		if err != nil {
			return nil, err
		}
	}

	remaining := 1 << 20
	if err := boundedConfigSize(object, 0, &remaining); err != nil {
		return nil, err
	}

	if config.Parser == model.ConfigParserYAML {
		node := configYAMLNode(object)

		var output bytes.Buffer

		encoder := yaml.NewEncoder(&output)
		encoder.SetIndent(2)

		if err := encoder.Encode(node); err != nil {
			return nil, err
		}

		if err := encoder.Close(); err != nil {
			return nil, err
		}

		return output.Bytes(), nil
	}

	return json.MarshalIndent(object, "", "  ")
}

func setConfigValue(current any, parts []string, value any, budget *int) (any, error) {
	*budget--
	if *budget < 0 {
		return nil, errors.New("configuration replacements exceed the work budget")
	}

	if len(parts) == 0 {
		return value, nil
	}

	head := parts[0]
	if current == nil {
		if head == "*" {
			return nil, errors.New("wildcard requires existing configuration entries")
		}

		if _, err := strconv.Atoi(head); err == nil {
			current = []any{}
		} else {
			current = map[string]any{}
		}
	}

	switch node := current.(type) {
	case map[string]any:
		if head == "*" {
			if len(node) == 0 {
				return nil, errors.New("wildcard requires existing configuration entries")
			}

			for key, child := range node {
				next, err := setConfigValue(child, parts[1:], value, budget)
				if err != nil {
					return nil, err
				}

				node[key] = next
			}
		} else {
			next, err := setConfigValue(node[head], parts[1:], value, budget)
			if err != nil {
				return nil, err
			}

			node[head] = next
		}

		return node, nil
	case []any:
		if head == "*" {
			if len(node) == 0 {
				return nil, errors.New("wildcard requires existing configuration entries")
			}

			for index, child := range node {
				next, err := setConfigValue(child, parts[1:], value, budget)
				if err != nil {
					return nil, err
				}

				node[index] = next
			}
		} else {
			index, err := strconv.Atoi(head)
			if err != nil || index < 0 || index >= 4096 {
				return nil, errors.New("config array indices must be between 0 and 4095")
			}

			if index >= len(node) {
				growth := index - len(node) + 1

				*budget -= growth
				if *budget < 0 {
					return nil, errors.New("configuration array expansion exceeds the work budget")
				}

				node = append(node, make([]any, growth)...)
			}

			next, err := setConfigValue(node[index], parts[1:], value, budget)
			if err != nil {
				return nil, err
			}

			node[index] = next
		}

		return node, nil
	default:
		return nil, errors.New("config path traverses a scalar value")
	}
}

func validateConfigYAML(node *yaml.Node, depth int) error {
	if depth > 32 {
		return errors.New("YAML configuration nesting exceeds 32 levels")
	}

	if node.Kind == yaml.AliasNode || node.Anchor != "" || node.Tag == "!!merge" {
		return errors.New("YAML anchors, aliases and merges are unsupported")
	}

	if node.Tag != "" && node.Tag != "!!map" && node.Tag != "!!seq" && node.Tag != "!!str" && node.Tag != "!!int" && node.Tag != "!!float" && node.Tag != "!!bool" && node.Tag != "!!null" {
		return errors.New("unsupported YAML scalar or custom tag")
	}

	for _, child := range node.Content {
		if err := validateConfigYAML(child, depth+1); err != nil {
			return err
		}
	}

	return nil
}

func configYAMLNode(value any) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode}

	switch item := value.(type) {
	case map[string]any:
		node.Kind, node.Tag = yaml.MappingNode, "!!map"

		keys := make([]string, 0, len(item))
		for key := range item {
			keys = append(keys, key)
		}

		sort.Strings(keys)

		for _, key := range keys {
			node.Content = append(node.Content, configYAMLNode(key), configYAMLNode(item[key]))
		}
	case []any:
		node.Kind, node.Tag = yaml.SequenceNode, "!!seq"
		for _, child := range item {
			node.Content = append(node.Content, configYAMLNode(child))
		}
	case json.Number:
		node.Value = item.String()

		node.Tag = "!!int"
		if strings.ContainsAny(node.Value, ".eE") {
			node.Tag = "!!float"
		}
	case string:
		node.Tag, node.Value = "!!str", item
	case bool:
		node.Tag, node.Value = "!!bool", strconv.FormatBool(item)
	default:
		node.Tag, node.Value = "!!null", "null"
	}

	return node
}
