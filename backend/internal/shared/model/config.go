package model

import (
	"errors"
	"strings"
	"unicode"
)

// ConfigKey validates the supported parser-specific path syntax before dispatch.
func ConfigKey(parser, key string) error {
	if key == "" || len(key) > 1024 || strings.ContainsFunc(key, unicode.IsControl) {
		return errors.New("invalid config replacement key")
	}

	if parser == "json" || parser == "yaml" {
		parts := strings.Split(key, ".")
		if len(parts) > 32 {
			return errors.New("config paths are limited to 32 components")
		}

		for _, part := range parts {
			if part == "" || strings.ContainsAny(part, "[]") || strings.Contains(part, "*") && part != "*" {
				return errors.New("config paths use dot-separated keys, numeric array indices and whole-component wildcards")
			}
		}
	}

	if parser == "ini" {
		if _, _, err := INIKey(key); err != nil {
			return err
		}
	}

	return nil
}

// INIKey supports section.key and [section.with.dots].key selectors.
func INIKey(key string) (string, string, error) {
	section, name, found := strings.Cut(key, ".")
	if strings.HasPrefix(key, "[") {
		end := strings.Index(key, "].")
		if end < 2 {
			return "", "", errors.New("invalid bracketed INI section")
		}

		section, name, found = key[1:end], key[end+2:], true
	}

	if !found {
		section, name = "", key
	}

	if name == "" || strings.ContainsAny(section, "[]\r\n") || strings.ContainsAny(name, "[]=\r\n") {
		return "", "", errors.New("invalid INI section or key")
	}

	return section, name, nil
}
