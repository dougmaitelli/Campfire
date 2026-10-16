package runtime

import (
	"bytes"
	"errors"
	"sort"
	"strings"

	"campfire/internal/shared/model"

	"gopkg.in/ini.v1"
)

func iniConfig(old []byte, values map[string]string) ([]byte, error) {
	config, err := ini.Load(old)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		section, name, err := model.INIKey(key)
		if err != nil {
			return nil, err
		}

		if strings.ContainsAny(values[key], "\r\n\x00") {
			return nil, errors.New("INI replacements must be single-line values")
		}

		config.Section(section).Key(name).SetValue(values[key])
	}

	var output bytes.Buffer
	if _, err := config.WriteTo(&output); err != nil {
		return nil, err
	}

	return output.Bytes(), nil
}

func textConfig(old []byte, values map[string]string) ([]byte, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	// Prefer the most specific prefix deterministically when prefixes overlap.
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) == len(keys[j]) {
			return keys[i] < keys[j]
		}

		return len(keys[i]) > len(keys[j])
	})

	for _, key := range keys {
		if strings.ContainsAny(values[key], "\r\n\x00") {
			return nil, errors.New("text replacements must be single lines")
		}
	}

	lines := strings.Split(string(old), "\n")
	for index, line := range lines {
		for _, key := range keys {
			if strings.HasPrefix(line, key) {
				lines[index] = values[key]
				break
			}
		}
	}

	size := len(lines) - 1
	for _, line := range lines {
		size += len(line)
		if size > 1<<20 {
			return nil, errors.New("rewritten text configuration exceeds 1 MiB")
		}
	}

	return []byte(strings.Join(lines, "\n")), nil
}
