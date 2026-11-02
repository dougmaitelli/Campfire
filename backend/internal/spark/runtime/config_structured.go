package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"

	"campfire/internal/shared/limits"
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

	budget := limits.DocumentNodes

	for _, key := range keys {
		if err := model.ConfigKey(config.Parser, key); err != nil {
			return nil, err
		}

		var err error

		object, err = setConfigValue(object, strings.Split(key, "."), parseConfigReplacement(values[key]), &budget, true)
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

func setConfigValue(current any, parts []string, value configReplacement, budget *int, present bool) (any, error) {
	*budget--
	if *budget < 0 {
		return nil, errors.New("configuration replacements exceed the work budget")
	}

	if _, missing := current.(missingConfigValue); missing {
		current, present = nil, false
	}

	if len(parts) == 0 {
		return value.forExisting(current, present), nil
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
				next, err := setConfigValue(child, parts[1:], value, budget, true)
				if err != nil {
					return nil, err
				}

				node[key] = next
			}
		} else {
			child, exists := node[head]

			next, err := setConfigValue(child, parts[1:], value, budget, exists)
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
				next, err := setConfigValue(child, parts[1:], value, budget, true)
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

			exists := index < len(node)
			if !exists {
				growth := index - len(node) + 1

				*budget -= growth
				if *budget < 0 {
					return nil, errors.New("configuration array expansion exceeds the work budget")
				}

				start := len(node)
				node = append(node, make([]any, growth)...)

				for slot := start; slot < len(node); slot++ {
					node[slot] = missingConfigValue{}
				}
			}

			next, err := setConfigValue(node[index], parts[1:], value, budget, exists)
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

// Array expansion creates gaps, which must remain distinguishable from existing
// null values until all replacements have been applied. Unfilled gaps emit null.
type missingConfigValue struct{}

func (missingConfigValue) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

// Config replacements follow the existing scalar type. Missing entries infer
// booleans and canonical JSON numbers, retaining their original precision.
type configReplacement struct {
	text     string
	isNumber bool
	isBool   bool
	boolean  bool
}

// Parse once per replacement, rather than once per wildcard match.
func parseConfigReplacement(value string) configReplacement {
	boolean, err := strconv.ParseBool(value)
	return configReplacement{text: value, isNumber: configNumber(value), isBool: err == nil, boolean: boolean}
}

func (value configReplacement) forExisting(existing any, present bool) any {
	switch existing.(type) {
	case bool:
		if value.isBool {
			return value.boolean
		}
	case json.Number:
		if value.isNumber {
			return json.Number(value.text)
		}
	default:
		if !present {
			if value.text == "true" || value.text == "false" {
				return value.text == "true"
			}

			if value.isNumber {
				return json.Number(value.text)
			}
		}
	}

	return value.text
}

func configNumber(value string) bool {
	return len(value) > 0 && (value[0] == '-' || value[0] >= '0' && value[0] <= '9') && strings.TrimSpace(value) == value && json.Valid([]byte(value))
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
