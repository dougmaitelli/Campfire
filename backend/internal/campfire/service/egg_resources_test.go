package service

import (
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestResourcePlaceholderAliases(t *testing.T) {
	for alias, name := range map[string]string{
		"server.build.cpu": "SERVER_CPU", "server.build.cpu_limit": "SERVER_CPU",
		"server.build.threads": "SERVER_THREADS", "server.build.disk": "SERVER_DISK",
		"server.build.disk_space": "SERVER_DISK", "server.build.swap": "SERVER_SWAP",
		"env.SERVER_CPU": "SERVER_CPU", "server.build.env.SERVER_SWAP": "SERVER_SWAP",
	} {
		report := &EmberPreview{}

		got := eggTemplate("{{"+alias+"}}", nil, "startup", report)
		if got != "{{"+name+"}}" || len(report.Errors) != 0 {
			t.Fatalf("%s: %q, %v", alias, got, report.Errors)
		}
	}
}

func TestResourceBuiltinsValidateAndCannotBeOverridden(t *testing.T) {
	for name := range model.BuiltinEnvironment() {
		ember := model.Ember{Schema: 1, Name: "Game", Images: map[string]string{"Game": "alpine:3.22"}, Startup: "echo {{" + name + "}}", WorkDir: "/home/container", User: "1000:1000"}
		if err := validateEmber(&ember); err != nil {
			t.Fatalf("builtin %s rejected: %v", name, err)
		}

		ember.Variables = []model.Variable{{Name: name, Editable: true}}
		if err := validateEmber(&ember); err == nil || !strings.Contains(err.Error(), "variable") {
			t.Fatalf("builtin override accepted for %s: %v", name, err)
		}
	}
}
