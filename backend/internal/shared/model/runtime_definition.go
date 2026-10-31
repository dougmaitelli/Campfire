package model

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// validate is shared with Ember authoring validation so both boundaries enforce
// the same container, configuration-file, and command safety constraints.
func (e RuntimeDefinition) validate(known map[string]string) error {
	if e.WorkDir == "" || !strings.HasPrefix(e.WorkDir, "/") || strings.ContainsAny(e.WorkDir, ",\x00\n") {
		return errors.New("work_dir must be an absolute container path")
	}

	if e.StartupShell != "" && e.StartupShell != "sh" && e.StartupShell != "bash" && e.StartupShell != "ash" {
		return errors.New("startup_shell must be sh, bash, or ash")
	}

	if !userID.MatchString(e.User) {
		return errors.New("runtime user must be a numeric non-root UID[:GID]")
	}

	if strings.ContainsRune(e.Startup, '\x00') {
		return errors.New("startup command must not contain null characters")
	}

	if _, err := Startup(e.Startup, known); err != nil {
		return err
	}

	if e.Installer != nil {
		if err := Image(e.Installer.Image); err != nil {
			return err
		}

		if e.Installer.Entrypoint != "bash" && e.Installer.Entrypoint != "sh" && e.Installer.Entrypoint != "ash" && e.Installer.Entrypoint != "/bin/bash" && e.Installer.Entrypoint != "/bin/sh" && e.Installer.Entrypoint != "/bin/ash" {
			return errors.New("installer entrypoint must be sh, bash, or ash")
		}
	}

	if _, err := StopSignal(e.Stop); err != nil {
		return err
	}

	for _, c := range e.Configs {
		if len(c.Strings) > 0 && c.Parser != ConfigParserJSON && c.Parser != ConfigParserYAML {
			return errors.New("string replacement types require JSON or YAML")
		}

		for _, key := range c.Strings {
			if _, ok := c.Values[key]; !ok {
				return errors.New("unknown JSON string replacement key")
			}
		}

		if c.Parser != ConfigParserProperties && c.Parser != ConfigParserJSON && c.Parser != ConfigParserYAML && c.Parser != ConfigParserINI && c.Parser != ConfigParserFile {
			return fmt.Errorf("unsupported config parser %q", c.Parser)
		}

		if !SafePath(c.Path) {
			return errors.New("config path must be relative and contained")
		}

		for k, v := range c.Values {
			if err := ConfigKey(c.Parser, k); err != nil {
				return err
			}

			if c.Parser == ConfigParserProperties && (strings.ContainsAny(k, "=\r\n") || strings.ContainsAny(v, "\r\n")) {
				return errors.New("invalid properties replacement")
			}

			if (c.Parser == ConfigParserINI || c.Parser == ConfigParserFile) && strings.ContainsAny(v, "\r\n\x00") {
				return errors.New("INI and file replacements must be single lines")
			}
		}
	}

	stop, err := Startup(e.Stop, known)
	if err != nil {
		return err
	}

	if strings.Contains(stop, "{{") || strings.Contains(stop, "}}") {
		return errors.New("malformed stop command placeholder")
	}

	if len(e.Stop) > 1024 || strings.ContainsAny(e.Stop, "\r\n\x00") {
		return errors.New("stop command must be a single line of at most 1024 bytes")
	}

	if e.Readiness != nil {
		if e.Ready != "" || len(e.Readiness.Any) == 0 || len(e.Readiness.Any) > 16 {
			return errors.New("use ready or 1–16 readiness matchers")
		}

		for _, match := range e.Readiness.Any {
			if len(match.Value) == 0 || len(match.Value) > 512 || strings.Contains(match.Value, "{{") {
				return errors.New("readiness matchers require 1–512 bytes without placeholders")
			}

			if match.Regex {
				if _, err := regexp.Compile(match.Value); err != nil {
					return fmt.Errorf("invalid readiness regex: %w", err)
				}
			}
		}
	}

	return nil
}
