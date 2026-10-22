package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
var userID = regexp.MustCompile(`^[1-9][0-9]*(?::[0-9]+)?$`)
var placeholder = regexp.MustCompile(`\{\{([A-Z][A-Z0-9_]*)\}\}`)

// Variable defines a game configuration value.
type Variable struct {
	Name       string              `json:"name"`
	Default    string              `json:"default"`
	Required   bool                `json:"required"`
	Pattern    string              `json:"pattern,omitempty"`
	Editable   bool                `json:"editable"`
	Hidden     bool                `json:"hidden,omitempty"`
	Validation *VariableValidation `json:"validation,omitempty"`
}

// Installer describes how to install game server files.
type Installer struct {
	Image      string `json:"image"`
	Entrypoint string `json:"entrypoint"`
	Script     string `json:"script"`
}

// ConfigFile describes configuration file replacements.
type ConfigFile struct {
	Path    string            `json:"path"`
	Parser  string            `json:"parser"`
	Values  map[string]string `json:"values"`
	Strings []string          `json:"strings,omitempty"`
	Create  bool              `json:"create,omitempty"`
}

// ReadinessMatch describes a literal or RE2-compatible line matcher.
type ReadinessMatch struct {
	Value string `json:"value"`
	Regex bool   `json:"regex,omitempty"`
}

// Readiness accepts any matching marker, optionally after removing terminal escapes.
type Readiness struct {
	Any       []ReadinessMatch `json:"any"`
	StripANSI bool             `json:"strip_ansi,omitempty"`
}

// RequiredPort identifies a container port required by a game.
type RequiredPort struct {
	Container   int    `json:"container"`
	Protocol    string `json:"protocol"`
	Description string `json:"description,omitempty"`
}

// PortDescriptions returns display metadata keyed by container port and protocol.
func (e Ember) PortDescriptions() map[string]string {
	labels := make(map[string]string)

	for _, port := range e.RequiredPorts {
		if port.Description != "" {
			labels[fmt.Sprintf("%d/%s", port.Container, port.Protocol)] = port.Description
		}
	}

	return labels
}

// Ember is an immutable, imported native game definition.
type Ember struct {
	ID              string            `json:"id"`
	Revision        string            `json:"revision,omitempty"`
	Schema          int               `json:"schema"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	ArtURL          string            `json:"artUrl,omitempty"`
	UpdateURL       string            `json:"updateUrl,omitempty"`
	Images          map[string]string `json:"images"`
	Startup         string            `json:"startup"`
	StartupCommands map[string]string `json:"startup_commands,omitempty"`
	StartupShell    string            `json:"startup_shell,omitempty"`
	WorkDir         string            `json:"work_dir"`
	User            string            `json:"user"`
	Variables       []Variable        `json:"variables"`
	Installer       *Installer        `json:"installer,omitempty"`
	Stop            string            `json:"stop,omitempty"`
	Ready           string            `json:"ready,omitempty"`
	Readiness       *Readiness        `json:"readiness,omitempty"`
	Configs         []ConfigFile      `json:"configs,omitempty"`
	RequiredPorts   []RequiredPort    `json:"required_ports,omitempty"`
	Origin          *EmberOrigin      `json:"origin,omitempty"`
}

// EmberHash identifies definition content without storing historical copies.
func EmberHash(ember Ember) string {
	ember.ID, ember.Revision, ember.UpdateURL, ember.Origin = "", "", "", nil
	data, _ := json.Marshal(ember)
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:])
}

// EmberOrigin retains attribution and conversion warnings for imported Eggs.
type EmberOrigin struct {
	Format    string   `json:"format"`
	Author    string   `json:"author"`
	UpdateURL string   `json:"update_url"`
	Warnings  []string `json:"warnings"`
}

// Validate checks a native Ember definition.
func (e Ember) Validate() error {
	if e.Schema != 1 || strings.TrimSpace(e.Name) == "" || len(e.Name) > 120 {
		return errors.New("Ember requires schema 1 and a name of 1–120 characters")
	}

	if err := ValidateArtURL(e.ArtURL); err != nil {
		return err
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

	if e.StartupShell != "" && e.StartupShell != "sh" && e.StartupShell != "bash" && e.StartupShell != "ash" {
		return errors.New("startup_shell must be sh, bash, or ash")
	}

	if !userID.MatchString(e.User) {
		return errors.New("runtime user must be a numeric non-root UID[:GID]")
	}

	seen := map[string]bool{}

	for _, v := range e.Variables {
		_, builtin := BuiltinEnvironment()[v.Name]
		if !envName.MatchString(v.Name) || seen[v.Name] || v.Name == "STARTUP" || builtin {
			return fmt.Errorf("invalid or reserved variable %q", v.Name)
		}

		seen[v.Name] = true
		if v.Validation != nil {
			if err := v.Validation.Validate(); err != nil {
				return fmt.Errorf("variable %s: %w", v.Name, err)
			}
		}

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

		if e.Installer.Entrypoint != "bash" && e.Installer.Entrypoint != "sh" && e.Installer.Entrypoint != "ash" && e.Installer.Entrypoint != "/bin/bash" && e.Installer.Entrypoint != "/bin/sh" && e.Installer.Entrypoint != "/bin/ash" {
			return errors.New("installer entrypoint must be sh, bash, or ash")
		}
	}

	for _, p := range e.RequiredPorts {
		if p.Container < 1 || p.Container > 65535 || (p.Protocol != "tcp" && p.Protocol != "udp") {
			return errors.New("invalid Ember required port")
		}

		if len(p.Description) > 80 || strings.ContainsFunc(p.Description, unicode.IsControl) {
			return errors.New("port descriptions must be at most 80 characters without control characters")
		}
	}

	if _, err := StopSignal(e.Stop); err != nil {
		return err
	}

	for _, c := range e.Configs {
		if len(c.Strings) > 0 && c.Parser != "json" && c.Parser != "yaml" {
			return errors.New("string replacement types require JSON or YAML")
		}

		for _, key := range c.Strings {
			if _, ok := c.Values[key]; !ok {
				return errors.New("unknown JSON string replacement key")
			}
		}

		if c.Parser != "properties" && c.Parser != "json" && c.Parser != "yaml" && c.Parser != "ini" && c.Parser != "file" {
			return fmt.Errorf("unsupported config parser %q", c.Parser)
		}

		if !SafePath(c.Path) {
			return errors.New("config path must be relative and contained")
		}

		for k, v := range c.Values {
			if err := ConfigKey(c.Parser, k); err != nil {
				return err
			}

			if c.Parser == "properties" && (strings.ContainsAny(k, "=\r\n") || strings.ContainsAny(v, "\r\n")) {
				return errors.New("invalid properties replacement")
			}

			if (c.Parser == "ini" || c.Parser == "file") && strings.ContainsAny(v, "\r\n\x00") {
				return errors.New("INI and file replacements must be single lines")
			}
		}
	}

	known := BuiltinEnvironment()

	for _, v := range e.Variables {
		known[v.Name] = ""
	}

	if _, err := Startup(e.Startup, known); err != nil {
		return err
	}

	if len(e.StartupCommands) > 32 || len(e.StartupCommands) > 0 && e.Startup != "" {
		return errors.New("use either startup or up to 32 named startup_commands")
	}

	for name, command := range e.StartupCommands {
		if strings.TrimSpace(name) == "" || len(name) > 120 || strings.ContainsAny(name, "\r\n\x00") || strings.TrimSpace(command) == "" {
			return errors.New("startup_commands requires nonempty names and commands")
		}

		if _, err := Startup(command, known); err != nil {
			return fmt.Errorf("startup command %q: %w", name, err)
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

// ValidateArtURL accepts HTTPS artwork or an asset hosted by this panel.
func ValidateArtURL(value string) error {
	if value == "" {
		return nil
	}

	parsed, err := url.Parse(value)
	if err != nil || len(value) > 2048 || strings.ContainsFunc(value, unicode.IsSpace) || strings.ContainsFunc(value, unicode.IsControl) {
		return errors.New("artUrl must be an HTTPS URL or a panel asset path of at most 2048 characters")
	}

	external := parsed.Scheme == "https" && parsed.Hostname() != ""

	local := parsed.Scheme == "" && parsed.Host == "" && strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") && !strings.Contains(value, "\\")
	if parsed.User != nil || (!external && !local) {
		return errors.New("artUrl must be an HTTPS URL or a panel asset path")
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

// ResolveStartup validates a server's selection and returns its stable key and command.
func (e Ember) ResolveStartup(name string) (string, string, error) {
	if len(e.StartupCommands) == 0 {
		if name != "" {
			return "", "", errors.New("this Ember has no named startup commands")
		}

		return "", e.Startup, nil
	}

	if name == "" && len(e.StartupCommands) == 1 {
		for key := range e.StartupCommands {
			name = key
		}
	}

	command, ok := e.StartupCommands[name]
	if !ok {
		return "", "", errors.New("select an available Ember startup command")
	}

	return name, command, nil
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
