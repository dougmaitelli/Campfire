package service

import (
	"regexp"
	"strings"

	"campfire/internal/shared/model"
)

var eggPlaceholder = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

func eggTemplate(text string, variables []model.Variable, field string, report *EmberPreview) string {
	known := map[string]bool{"SERVER_PORT": true, "SERVER_IP": true, "SERVER_MEMORY": true}
	for _, variable := range variables {
		known[variable.Name] = true
	}

	aliases := map[string]string{
		"server.build.default.port": "SERVER_PORT", "server.allocations.default.port": "SERVER_PORT",
		"server.build.default.ip": "SERVER_IP", "server.allocations.default.ip": "SERVER_IP",
		"server.build.memory": "SERVER_MEMORY", "server.build.memory_limit": "SERVER_MEMORY",
	}

	result := eggPlaceholder.ReplaceAllStringFunc(text, func(match string) string {
		name := strings.TrimSpace(eggPlaceholder.FindStringSubmatch(match)[1])
		if alias, ok := aliases[name]; ok {
			name = alias
		} else {
			for _, prefix := range []string{"env.", "server.build.env.", "server.build.environment.", "server.environment."} {
				if strings.HasPrefix(name, prefix) {
					name = strings.TrimPrefix(name, prefix)
					break
				}
			}
		}

		if !known[name] {
			report.fail(field, "Unsupported placeholder "+match+".")
			return match
		}

		return "{{" + name + "}}"
	})
	if strings.Contains(result, "{{") || strings.Contains(result, "}}") {
		remainder := eggPlaceholder.ReplaceAllString(result, "")
		if strings.Contains(remainder, "{{") || strings.Contains(remainder, "}}") {
			report.fail(field, "Malformed placeholder syntax.")
		}
	}

	return result
}
