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
	Tags            []string          `json:"tags"`
	Features        []string          `json:"features"`
	DockerImages    map[string]string `json:"docker_images"`
	Images          []string          `json:"images"`
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
	Variables          []eggVariable   `json:"variables"`
	ConfigFrom         json.RawMessage `json:"config_from"`
	CopyScriptFrom     json.RawMessage `json:"copy_script_from"`
	ScriptIsPrivileged bool            `json:"script_is_privileged"`
	ForceOutgoingIP    bool            `json:"force_outgoing_ip"`
	Mounts             json.RawMessage `json:"mounts"`
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

func convertEgg(data []byte, options ImportOptions, report *EmberPreview) model.Ember {
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
		keys := sortedKeys(egg.StartupCommands)
		report.StartupCommands = keys

		selected := options.StartupCommand
		if selected == "" && len(keys) == 1 {
			selected = keys[0]
		}

		if selected == "" {
			report.fail("startup_command", "Select one of the exported startup commands.")
		} else if command, ok := egg.StartupCommands[selected]; ok {
			ember.Startup = command

			if len(keys) > 1 {
				report.warn("startup_commands", "Only the selected startup command is retained in this Ember.")
			}
		} else {
			report.fail("startup_command", "The selected startup command does not exist.")
		}

		if egg.Startup != "" {
			report.fail("startup", "PLCN_v3 uses startup_commands, not startup.")
		}
	} else {
		ember.Startup = egg.Startup
		if len(egg.StartupCommands) > 0 || options.StartupCommand != "" {
			report.fail("startup_commands", "Startup selection is only supported for PLCN_v3.")
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
		if slices.Contains([]string{"eula", "java_version", "steam_disk_space", "pid_limit"}, feature) {
			report.warn("features", fmt.Sprintf("%s panel integration is unavailable; handle it manually. Campfire keeps its own resource and process limits.", feature))
		} else {
			report.fail("features", fmt.Sprintf("Unsupported Egg feature %q.", feature))
		}
	}

	for index, variable := range egg.Variables {
		path := fmt.Sprintf("variables[%d]", index)
		if variable.Viewable == nil || !*variable.Viewable {
			report.fail(path+".user_viewable", "Hidden variables are unsupported; values must not become visible through conversion.")
		}

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
		if !converted.Editable {
			report.warn(path+".user_editable", "Non-editable variables are pinned to their defaults for everyone, including administrators.")
		}

		if converted.Pattern != "" {
			report.warn(path+".rules", "Regex matching uses Go RE2; review Unicode character and case-folding differences from PCRE.")
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

	ember.Stop = eggTemplate(ember.Stop, ember.Variables, "config.stop", report)
	if strings.Contains(ember.Stop, "{{") {
		report.fail("config.stop", "Variable stop commands are unsupported; use a literal command or signal.")
	}

	convertEggProcess(egg.Config.Startup, egg.Config.Logs, &ember, report)
	ember.Configs = convertEggFiles(egg.Config.Files, ember.Variables, report)
	report.warn("runtime", "Uses /home/container and UID 1000:1000. Startup is supplied as STARTUP and sh -c arguments; the image ENTRYPOINT remains active. Review image hooks, shell and script requirements before starting.")
	report.warn("ports", "Egg exports do not specify Campfire port mappings. Configure all TCP/UDP mappings explicitly; SERVER_PORT is the first container port and SERVER_IP is 0.0.0.0 inside the container.")

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
		var done string
		if !emptyEggValue(process.Done) {
			if err := json.Unmarshal(process.Done, &done); err != nil {
				var alternatives []string
				if err := json.Unmarshal(process.Done, &alternatives); err != nil || len(alternatives) > 1 {
					report.fail("config.startup.done", "Only one literal readiness marker is supported.")
				} else if len(alternatives) == 1 {
					done = alternatives[0]
				}
			}
		}

		if strings.HasPrefix(done, "/") || strings.Contains(done, "{{") {
			report.fail("config.startup.done", "Readiness regexes and placeholders are unsupported.")
		}

		if len(process.UserInteraction) > 0 || process.StripANSI {
			report.fail("config.startup", "Interactive startup prompts and ANSI-stripping detection are unsupported.")
		}

		ember.Ready = done
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

		if file.Parser != "properties" && file.Parser != "json" {
			report.fail(path+".parser", "Only properties and top-level JSON replacements are supported.")
			continue
		}

		config := model.ConfigFile{Path: name, Parser: file.Parser, Values: map[string]string{}}
		for _, key := range sortedKeys(file.Find) {
			if file.Parser == "json" && strings.ContainsAny(key, ".[]*") {
				report.fail(path+".find."+key, "Nested paths, arrays and wildcards are unsupported.")
				continue
			}

			value := file.Find[key]

			text, err := eggScalar(value)
			if err != nil || bytes.Equal(value, []byte("null")) {
				report.fail(path+".find."+key, "Only non-null scalar replacements are supported; conditional replacements are unsupported.")
				continue
			}

			if file.Parser == "json" && len(value) > 0 && value[0] == '"' {
				config.Strings = append(config.Strings, key)
			}

			config.Values[key] = eggTemplate(text, variables, path+".find."+key, report)
		}

		configs = append(configs, config)
	}

	if len(configs) > 0 {
		report.warn("config.files", "Configuration files must already exist (create them in the installer). Campfire supports simple key=value properties and top-level JSON; it does not emulate every Wings parser behavior.")
	}

	return configs
}
