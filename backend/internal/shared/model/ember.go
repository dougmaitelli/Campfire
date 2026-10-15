package model

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
var userID = regexp.MustCompile(`^[1-9][0-9]*(?::[0-9]+)?$`)
var placeholder = regexp.MustCompile(`\{\{([A-Z][A-Z0-9_]*)\}\}`)

// Variable defines a game configuration value.
type Variable struct {
	Name     string `json:"name"`
	Default  string `json:"default"`
	Required bool   `json:"required"`
	Pattern  string `json:"pattern,omitempty"`
	Editable bool   `json:"editable"`
}

// Installer describes how to install game server files.
type Installer struct {
	Image      string `json:"image"`
	Entrypoint string `json:"entrypoint"`
	Script     string `json:"script"`
}

// ConfigFile describes configuration file replacements.
type ConfigFile struct {
	Path   string            `json:"path"`
	Parser string            `json:"parser"`
	Values map[string]string `json:"values"`
}

// RequiredPort identifies a container port required by a game.
type RequiredPort struct {
	Container int    `json:"container"`
	Protocol  string `json:"protocol"`
}

// Ember is an immutable, imported native game definition.
type Ember struct {
	ID            string            `json:"id"`
	Schema        int               `json:"schema"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Images        map[string]string `json:"images"`
	Startup       string            `json:"startup"`
	WorkDir       string            `json:"work_dir"`
	User          string            `json:"user"`
	Variables     []Variable        `json:"variables"`
	Installer     *Installer        `json:"installer,omitempty"`
	Stop          string            `json:"stop,omitempty"`
	Ready         string            `json:"ready,omitempty"`
	Configs       []ConfigFile      `json:"configs,omitempty"`
	RequiredPorts []RequiredPort    `json:"required_ports,omitempty"`
}

// Validate checks a native Ember definition.
func (e Ember) Validate() error {
	if e.Schema != 1 || strings.TrimSpace(e.Name) == "" || len(e.Name) > 120 {
		return errors.New("Ember requires schema 1 and a name of 1–120 characters")
	}

	if len(e.Images) == 0 {
		return errors.New("Ember requires at least one image")
	}

	for _, image := range e.Images {
		if err := Image(image); err != nil {
			return err
		}
	}

	if e.WorkDir == "" || !strings.HasPrefix(e.WorkDir, "/") || strings.ContainsAny(e.WorkDir, ",\x00\n") {
		return errors.New("work_dir must be an absolute container path")
	}

	if !userID.MatchString(e.User) {
		return errors.New("runtime user must be a numeric non-root UID[:GID]")
	}

	seen := map[string]bool{}
	for _, v := range e.Variables {
		if !envName.MatchString(v.Name) || seen[v.Name] || v.Name == "STARTUP" || v.Name == "SERVER_PORT" || v.Name == "SERVER_IP" {
			return fmt.Errorf("invalid or reserved variable %q", v.Name)
		}

		seen[v.Name] = true
		if len(v.Pattern) > 512 {
			return errors.New("variable pattern too long")
		}

		if v.Pattern != "" {
			if _, err := regexp.Compile(v.Pattern); err != nil {
				return fmt.Errorf("variable %s: %w", v.Name, err)
			}
		}
	}

	if e.Installer != nil {
		if err := Image(e.Installer.Image); err != nil {
			return err
		}

		if e.Installer.Entrypoint != "bash" && e.Installer.Entrypoint != "sh" && e.Installer.Entrypoint != "/bin/bash" && e.Installer.Entrypoint != "/bin/sh" {
			return errors.New("installer entrypoint must be sh or bash")
		}
	}

	for _, p := range e.RequiredPorts {
		if p.Container < 1 || p.Container > 65535 || (p.Protocol != "tcp" && p.Protocol != "udp") {
			return errors.New("invalid Ember required port")
		}
	}

	if strings.HasPrefix(e.Stop, "^") && e.Stop != "^C" && e.Stop != "^SIGINT" && e.Stop != "^SIGTERM" {
		return errors.New("supported stop signals are ^C, ^SIGINT, and ^SIGTERM")
	}

	for _, c := range e.Configs {
		if c.Parser != "properties" && c.Parser != "json" {
			return fmt.Errorf("unsupported config parser %q", c.Parser)
		}

		if !SafePath(c.Path) {
			return errors.New("config path must be relative and contained")
		}

		for k, v := range c.Values {
			if c.Parser == "json" && strings.Contains(k, ".") {
				return errors.New("nested JSON config keys are unsupported")
			}

			if c.Parser == "properties" && (strings.ContainsAny(k, "=\r\n") || strings.ContainsAny(v, "\r\n")) {
				return errors.New("invalid properties replacement")
			}
		}
	}

	known := map[string]string{"SERVER_PORT": "", "SERVER_IP": ""}
	for _, v := range e.Variables {
		known[v.Name] = ""
	}

	if _, err := Startup(e.Startup, known); err != nil {
		return err
	}

	return nil
}

// Image validates a container image reference.
func Image(s string) error {
	if s == "" || strings.HasPrefix(s, "-") || strings.ContainsAny(s, " \t\r\n\x00") {
		return errors.New("invalid container image")
	}

	return nil
}

// SafePath checks that a configuration path is relative and contained.
func SafePath(s string) bool {
	if s == "" || strings.HasPrefix(s, "/") || strings.ContainsAny(s, "\\\x00") {
		return false
	}

	for _, p := range strings.Split(s, "/") {
		if p == ".." {
			return false
		}
	}

	return true
}

// Startup validates environment placeholders in a startup command.
func Startup(s string, env map[string]string) (string, error) {
	var missing string

	out := placeholder.ReplaceAllStringFunc(s, func(m string) string {
		k := placeholder.FindStringSubmatch(m)[1]
		if _, ok := env[k]; !ok {
			missing = k
		}

		return "${" + k + "}"
	})

	if missing != "" {
		return "", fmt.Errorf("unknown startup variable %s", missing)
	}

	return out, nil
}
