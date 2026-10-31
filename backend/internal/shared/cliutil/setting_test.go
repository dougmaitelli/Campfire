package cliutil

import (
	"flag"
	"os"
	"strings"
	"testing"
)

func TestStringSettingPrecedence(t *testing.T) {
	setting := Setting{FlagName: "example", EnvironmentName: "CAMPFIRE_TEST_SETTING"}
	for _, test := range []struct {
		name        string
		environment string
		unset       bool
		args        []string
		want        string
	}{
		{name: "fallback", unset: true, want: "fallback"},
		{name: "environment", environment: "environment", want: "environment"},
		{name: "explicit empty environment", want: ""},
		{name: "flag overrides environment", environment: "environment", args: []string{"--" + setting.FlagName, "flag"}, want: "flag"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(setting.EnvironmentName, test.environment)

			if test.unset {
				if err := os.Unsetenv(setting.EnvironmentName); err != nil {
					t.Fatal(err)
				}
			}

			flags := flag.NewFlagSet(test.name, flag.ContinueOnError)

			value := setting.String(flags, "fallback", "example setting")
			if err := flags.Parse(test.args); err != nil {
				t.Fatal(err)
			}

			if *value != test.want {
				t.Fatal("incorrect configuration precedence", *value, test.want)
			}

			if !strings.Contains(flags.Lookup(setting.FlagName).Usage, setting.EnvironmentName) {
				t.Fatal("flag help omitted the environment name")
			}
		})
	}
}

func TestBoolSettingValidationAndOverride(t *testing.T) {
	setting := Setting{FlagName: "example", EnvironmentName: "CAMPFIRE_TEST_BOOL_SETTING"}
	t.Setenv(setting.EnvironmentName, "true")

	flags := flag.NewFlagSet("boolean", flag.ContinueOnError)

	value, err := setting.Bool(flags, false, "example setting")
	if err != nil || !*value {
		t.Fatal("environment default was not used", value, err)
	}

	if err := flags.Parse([]string{"--" + setting.FlagName + "=false"}); err != nil || *value {
		t.Fatal("explicit flag did not override the environment", value, err)
	}

	t.Setenv(setting.EnvironmentName, "invalid")

	if _, err := setting.Bool(flag.NewFlagSet("invalid", flag.ContinueOnError), false, "example setting"); err == nil || err.Error() != setting.EnvironmentName+" must be a boolean" {
		t.Fatal("invalid environment value was not reported", err)
	}
}

func TestFlagOnlySettings(t *testing.T) {
	flags := flag.NewFlagSet("flag-only", flag.ContinueOnError)
	textSetting := Setting{FlagName: "text"}
	boolSetting := Setting{FlagName: "enabled"}
	text := textSetting.String(flags, "fallback", "text setting")

	enabled, err := boolSetting.Bool(flags, true, "boolean setting")
	if err != nil || *text != "fallback" || !*enabled {
		t.Fatal("flag-only defaults were not preserved", text, enabled, err)
	}

	if err := flags.Parse([]string{"--" + textSetting.FlagName, "override", "--" + boolSetting.FlagName + "=false"}); err != nil || *text != "override" || *enabled {
		t.Fatal("flag-only values could not be overridden", text, enabled, err)
	}

	if flags.Lookup(textSetting.FlagName).Usage != "text setting" || flags.Lookup(boolSetting.FlagName).Usage != "boolean setting" {
		t.Fatal("flag-only help included an environment suffix")
	}
}

func TestEnvironmentOnlySetting(t *testing.T) {
	setting := Setting{EnvironmentName: "CAMPFIRE_TEST_ENV_SETTING"}
	t.Setenv(setting.EnvironmentName, "false")

	if value, err := setting.BoolValue(true); err != nil || value {
		t.Fatal("environment-only boolean was not parsed", value, err)
	}

	t.Setenv(setting.EnvironmentName, "")

	if value := setting.Value("fallback"); value != "" {
		t.Fatal("explicit empty environment value was ignored", value)
	}

	if _, err := setting.BoolValue(true); err == nil {
		t.Fatal("empty boolean environment value was accepted")
	}
}
