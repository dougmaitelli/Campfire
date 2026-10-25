package model

import (
	"strings"
	"testing"
)

func TestServerStartupOverrideValidation(t *testing.T) {
	e := Ember{Startup: "default", Variables: []Variable{{Name: "PORT"}}}
	for _, command := range []string{" ", "echo\x00bad", strings.Repeat("a", 16385), "echo {{MISSING}}"} {
		if _, _, err := e.ServerStartup(ServerConfiguration{StartupOverride: command}); err == nil {
			t.Fatalf("invalid override accepted: %.40q", command)
		}
	}

	for _, command := range []string{"", "echo {{PORT}}", "echo custom"} {
		_, got, err := e.ServerStartup(ServerConfiguration{StartupOverride: command})

		want := command
		if want == "" {
			want = e.Startup
		}

		if err != nil || got != want {
			t.Fatal("wrong effective command", got, err)
		}
	}
}
