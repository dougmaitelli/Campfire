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

// ExtendedBehavior identifies definitions requiring the extended Ember capability.
func (e Ember) ExtendedBehavior() bool {
	for _, name := range []string{"SERVER_CPU", "SERVER_THREADS", "SERVER_DISK", "SERVER_SWAP"} {
		if strings.Contains(e.Startup, name) || strings.Contains(e.Stop, name) || e.Installer != nil && strings.Contains(e.Installer.Script, name) {
			return true
		}

		for _, config := range e.Configs {
			for _, value := range config.Values {
				if strings.Contains(value, name) {
					return true
				}
			}
		}
	}

	if strings.HasPrefix(e.Stop, "^") && e.Stop != "^C" && e.Stop != "^SIGINT" && e.Stop != "^SIGTERM" {
		return true
	}

	if e.Readiness != nil || strings.Contains(e.Stop, "{{") {
		return true
	}

	for _, config := range e.Configs {
		if config.Create || config.Parser != "json" && config.Parser != "properties" {
			return true
		}

		for key := range config.Values {
			if config.Parser == "json" && strings.ContainsAny(key, ".*") {
				return true
			}
		}
	}

	return false
}
