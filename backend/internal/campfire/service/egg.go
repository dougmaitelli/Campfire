package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"campfire/internal/shared/model"
)

// eggList accepts PHP's empty-object representation of an empty exported list.
// Nonempty lists still use the strict decoder, including their nested fields.
type eggList[T any] []T

func (list *eggList[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("{}")) {
		*list = nil
		return nil
	}

	return strictDefinition(data, (*[]T)(list))
}

type eggDefinition struct {
	Comment json.RawMessage `json:"_comment"`
	Meta    struct {
		Version   string `json:"version"`
		UpdateURL string `json:"update_url"`
	} `json:"meta"`
	ExportedAt      string            `json:"exported_at"`
	Name            string            `json:"name"`
	Author          string            `json:"author"`
	UUID            string            `json:"uuid"`
	Description     string            `json:"description"`
	Icon            string            `json:"icon"`
	Tags            eggList[string]   `json:"tags"`
	Features        eggList[string]   `json:"features"`
	DockerImages    map[string]string `json:"docker_images"`
	Images          eggList[string]   `json:"images"`
	Image           string            `json:"image"`
	FileDenylist    json.RawMessage   `json:"file_denylist"`
	Startup         string            `json:"startup"`
	StartupCommands map[string]string `json:"startup_commands"`
	Config          struct {
		Files   json.RawMessage `json:"files"`
		Startup json.RawMessage `json:"startup"`
		Logs    json.RawMessage `json:"logs"`
		Stop    string          `json:"stop"`
	} `json:"config"`
	Scripts struct {
		Installation *struct {
			Script     *string `json:"script"`
			Container  string  `json:"container"`
			Entrypoint string  `json:"entrypoint"`
		} `json:"installation"`
	} `json:"scripts"`
	Variables          eggList[eggVariable] `json:"variables"`
	ConfigFrom         json.RawMessage      `json:"config_from"`
	CopyScriptFrom     json.RawMessage      `json:"copy_script_from"`
	ScriptIsPrivileged bool                 `json:"script_is_privileged"`
	ForceOutgoingIP    bool                 `json:"force_outgoing_ip"`
	Mounts             json.RawMessage      `json:"mounts"`
}

type eggVariable struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Environment string          `json:"env_variable"`
	Default     json.RawMessage `json:"default_value"`
	Viewable    *bool           `json:"user_viewable"`
	Editable    bool            `json:"user_editable"`
	Rules       json.RawMessage `json:"rules"`
	FieldType   json.RawMessage `json:"field_type"`
	Sort        int             `json:"sort"`
}

func convertEgg(data []byte, report *EmberPreview) model.Ember {
	var egg eggDefinition
	if err := strictDefinition(data, &egg); err != nil {
		report.fail("definition", err.Error())
		return model.Ember{}
	}

	ember := model.Ember{Schema: 1, Name: egg.Name, Description: egg.Description, Images: egg.DockerImages, WorkDir: "/home/container", User: "1000:1000", Variables: []model.Variable{}, Stop: egg.Config.Stop, Origin: &model.EmberOrigin{Format: egg.Meta.Version, Author: egg.Author, UpdateURL: egg.Meta.UpdateURL, Warnings: []string{}}}
	if egg.Meta.Version == "PTDL_v1" {
		if len(egg.DockerImages) > 0 {
			report.fail("docker_images", "PTDL_v1 uses image or images, not docker_images.")
		}

		ember.Images = map[string]string{}
		for _, image := range egg.Images {
			ember.Images[image] = image
		}

		if egg.Image != "" {
			ember.Images[egg.Image] = egg.Image
		}
	} else if len(egg.Images) > 0 {
		report.fail("images", "This Egg version requires docker_images.")
	}

	if egg.Meta.Version != "PTDL_v1" && egg.Image != "" {
		report.warn("image", "Legacy image/icon metadata is not used; docker_images supplies the runtime images.")
	}

	if egg.Meta.Version == "PLCN_v3" {
		ember.StartupCommands = egg.StartupCommands
		if len(ember.StartupCommands) == 0 {
			report.fail("startup_commands", "Export at least one named startup command.")
		}

		if egg.Startup != "" {
			report.fail("startup", "PLCN_v3 uses startup_commands, not startup.")
		}
	} else {
		ember.Startup = egg.Startup
		if len(egg.StartupCommands) > 0 {
			report.fail("startup_commands", "Named startup commands require PLCN_v3.")
		}
	}

	if !emptyEggValue(egg.FileDenylist) {
		report.fail("file_denylist", "File denylists are not enforced by Campfire; this Egg cannot be imported safely.")
	}

	if !emptyEggValue(egg.Mounts) {
		report.fail("mounts", "External mounts are unsupported.")
	}

	for field, raw := range map[string]json.RawMessage{"config_from": egg.ConfigFrom, "copy_script_from": egg.CopyScriptFrom} {
		if !emptyEggValue(raw) && string(raw) != "0" {
			report.fail(field, "Export inherited settings as a self-contained Egg.")
		}
	}

	if egg.ScriptIsPrivileged {
		report.fail("script_is_privileged", "Privileged installers are unsupported.")
	}

	if egg.ForceOutgoingIP {
		report.fail("force_outgoing_ip", "Forced outgoing addresses are unsupported.")
	}

	for _, feature := range egg.Features {
		switch feature {
		case "eula":
			report.warn("features.eula", "Before starting a Minecraft server, read its EULA and set eula=true in eula.txt if you agree. Campfire does not accept it for you.")
		case "java_version":
			report.warn("features.java_version", "Choose a Java image compatible with your Minecraft version when creating the server. Automatic Java selection is unavailable.")
		case "pid_limit":
			report.note("features.pid_limit", "Campfire applies its own process limits.")
		case "steam_disk_space":
			report.warn("features.steam_disk_space", "Check available disk space before installing; automatic Steam disk-space checks are unavailable.")
		default:
			report.fail("features", fmt.Sprintf("Unsupported Egg feature %q.", feature))
		}
	}

	for index, variable := range egg.Variables {
		path := fmt.Sprintf("variables[%d]", index)

		value, err := eggScalar(variable.Default)
		if err != nil {
			report.fail(path+".default_value", "Variable defaults must be scalar values.")
		}

		converted, err := convertEggRules(variable.Rules)
		if err != nil {
			report.fail(path+".rules", err.Error())
		}

		converted.Name = variable.Environment
		converted.Default = value

		converted.Editable = variable.Editable

		converted.Hidden = variable.Viewable == nil || !*variable.Viewable
		if !converted.Editable {
			report.warn(path+".user_editable", "Non-editable variables are pinned to their defaults for everyone, including administrators.")
		}

		if converted.Pattern != "" {
			report.note(path+".rules", "Variable validation was converted to Go RE2. Unicode character and case-folding behavior can differ from PCRE.")
		}

		if converted.Validation != nil && converted.Validation.Format == "url" {
			report.note(path+".rules", "URL values are validated as absolute URLs with a host using Go's URL parser.")
		}

		if converted.Default != "" && converted.ValidateValue(converted.Default) != nil {
			report.fail(path+".default_value", "Default value does not satisfy its validation rules.")
		}

		if converted.Default == "" && converted.Required && !converted.Editable {
			report.fail(path+".default_value", "A required non-editable variable needs a default value.")
		}

		ember.Variables = append(ember.Variables, converted)
	}

	if install := egg.Scripts.Installation; install != nil && install.Script != nil && strings.TrimSpace(*install.Script) != "" {
		ember.Installer = &model.Installer{Image: install.Container, Entrypoint: install.Entrypoint, Script: *install.Script}
		if strings.Contains(*install.Script, "{{") {
			report.fail("scripts.installation.script", "Install scripts must use shell environment variables, not panel placeholders.")
		}
	}

	ember.Startup = eggTemplate(ember.Startup, ember.Variables, "startup", report)
	for name, command := range ember.StartupCommands {
		ember.StartupCommands[name] = eggTemplate(command, ember.Variables, "startup_commands."+name, report)
	}

	ember.StartupShell = "bash"

	ember.Stop = eggTemplate(ember.Stop, ember.Variables, "config.stop", report)

	convertEggProcess(egg.Config.Startup, egg.Config.Logs, &ember, report)
	ember.Configs = convertEggFiles(egg.Config.Files, ember.Variables, report)
	report.note("runtime", "Uses /home/container and UID 1000:1000. Startup is supplied as STARTUP and bash -c arguments; the image ENTRYPOINT remains active. The runtime image must include Bash.")
	report.warn("ports", "Configure the game's TCP/UDP ports when creating the server; Egg exports do not include port mappings.")

	for _, issue := range report.Warnings {
		ember.Origin.Warnings = append(ember.Origin.Warnings, issue.Field+": "+issue.Message)
	}

	return ember
}

func emptyEggValue(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value == "" || value == "null" || value == "[]" || value == "{}" || value == `""`
}

func eggScalar(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}

	if bytes.Equal(raw, []byte("true")) || bytes.Equal(raw, []byte("false")) {
		return string(raw), nil
	}

	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String(), nil
	}

	return "", fmt.Errorf("expected scalar")
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// eggObject accepts nested JSON strings used by both panels and objects used by YAML exports.
func eggObject(raw json.RawMessage, target any) error {
	if emptyEggValue(raw) {
		raw = []byte("{}")
	}

	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}

		if strings.TrimSpace(text) == "" {
			text = "{}"
		}

		normalized, err := definitionDocument([]byte(text))
		if err != nil {
			return err
		}

		raw = normalized
	}

	return strictDefinition(raw, target)
}

func convertEggProcess(startup, logs json.RawMessage, ember *model.Ember, report *EmberPreview) {
	var process struct {
		Done            json.RawMessage   `json:"done"`
		UserInteraction []json.RawMessage `json:"userInteraction"`
		StripANSI       bool              `json:"strip_ansi"`
	}
	if err := eggObject(startup, &process); err != nil {
		report.fail("config.startup", "Unsupported startup detection settings.")
	} else {
		var done []string

		if !emptyEggValue(process.Done) {
			var literal string
			if err := json.Unmarshal(process.Done, &literal); err == nil {
				if literal != "" {
					done = []string{literal}
				}
			} else if err := json.Unmarshal(process.Done, &done); err != nil {
				report.fail("config.startup.done", "Readiness must be a string or array of strings.")
			}
		}

		if len(process.UserInteraction) > 0 {
			for _, raw := range process.UserInteraction {
				var marker string
				if err := json.Unmarshal(raw, &marker); err != nil {
					report.fail("config.startup.userInteraction", "Console prompt markers must be strings.")
				}
			}

			report.note("config.startup.userInteraction", "Console prompt highlighting is not imported. Messages remain visible in the server console; respond there or edit the server files as needed.")
		}

		if len(done) > 16 {
			report.fail("config.startup.done", "At most 16 readiness markers are supported.")
		}

		for _, value := range done {
			if value == "" || len(value) > 512 || strings.Contains(value, "{{") {
				report.fail("config.startup.done", "Readiness markers require 1–512 bytes without placeholders.")
			}
		}

		if len(done) == 1 && !process.StripANSI && !strings.HasPrefix(done[0], "/") && !strings.Contains(done[0], "{{") {
			ember.Ready = done[0]
		} else if len(done) > 0 {
			ember.Readiness = &model.Readiness{StripANSI: process.StripANSI}

			for _, value := range done {
				match := model.ReadinessMatch{Value: value}
				if strings.HasPrefix(value, "/") {
					pattern, err := eggRegex(value)
					if err != nil {
						report.fail("config.startup.done", err.Error())
						continue
					}

					match.Value, match.Regex = pattern, true
				}

				ember.Readiness.Any = append(ember.Readiness.Any, match)
			}
		}
	}

	var logging struct {
		Custom   bool   `json:"custom"`
		Location string `json:"location"`
	}
	if err := eggObject(logs, &logging); err != nil || logging.Custom {
		report.fail("config.logs", "Custom log file tailing is unsupported; use container stdout/stderr.")
	}
}

func convertEggFiles(raw json.RawMessage, variables []model.Variable, report *EmberPreview) []model.ConfigFile {
	var files map[string]json.RawMessage
	if err := eggObject(raw, &files); err != nil {
		report.fail("config.files", "Invalid configuration file definitions.")
		return nil
	}

	configs := []model.ConfigFile{}

	for _, name := range sortedKeys(files) {
		path := "config.files." + name

		var file struct {
			Parser string                     `json:"parser"`
			Find   map[string]json.RawMessage `json:"find"`
		}
		if err := strictDefinition(files[name], &file); err != nil {
			report.fail(path, "Unknown or invalid configuration settings.")
			continue
		}

		if file.Parser == "yml" {
			file.Parser = "yaml"
		}

		if !slices.Contains([]string{"properties", "json", "yaml", "ini", "file"}, file.Parser) {
			report.fail(path+".parser", "Supported parsers are properties, JSON, YAML, INI and file.")
			continue
		}

		config := model.ConfigFile{Path: name, Parser: file.Parser, Values: map[string]string{}, Create: true}
		for _, key := range sortedKeys(file.Find) {
			if err := model.ConfigKey(file.Parser, key); err != nil {
				report.fail(path+".find."+key, err.Error())
				continue
			}

			value := file.Find[key]

			text, err := eggScalar(value)
			if err != nil || bytes.Equal(value, []byte("null")) {
				report.fail(path+".find."+key, "Only non-null scalar replacements are supported; conditional replacements are unsupported.")
				continue
			}

			if (file.Parser == "json" || file.Parser == "yaml") && len(value) > 0 && value[0] == '"' {
				config.Strings = append(config.Strings, key)
			}

			config.Values[key] = eggTemplate(text, variables, path+".find."+key, report)
		}

		configs = append(configs, config)
	}

	if len(configs) > 0 {
		report.note("config.files", "Missing configuration files are created. JSON/YAML rewrites normalize formatting; YAML anchors, aliases, merges and custom tags are rejected. Parser paths and wildcard replacements can differ from Wings.")
	}

	return configs
}
