package cliutil

import (
	"flag"
	"fmt"
	"strconv"
)

// Setting describes a flag, an environment variable, or their pairing.
type Setting struct {
	FlagName        string
	EnvironmentName string
}

// Value returns the environment value when set, otherwise the fallback.
func (s Setting) Value(fallback string) string {
	if s.EnvironmentName == "" {
		return fallback
	}

	return Environment(s.EnvironmentName, fallback)
}

// String registers a string flag using the environment value as its default.
func (s Setting) String(flags *flag.FlagSet, fallback, usage string) *string {
	return flags.String(s.FlagName, s.Value(fallback), s.usage(usage))
}

// Bool registers a boolean flag after validating its environment default.
func (s Setting) Bool(flags *flag.FlagSet, fallback bool, usage string) (*bool, error) {
	value, err := s.BoolValue(fallback)
	if err != nil {
		return nil, err
	}

	return flags.Bool(s.FlagName, value, s.usage(usage)), nil
}

// BoolValue validates a boolean environment value without registering a flag.
func (s Setting) BoolValue(fallback bool) (bool, error) {
	value, err := strconv.ParseBool(s.Value(strconv.FormatBool(fallback)))
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", s.EnvironmentName)
	}

	return value, nil
}

func (s Setting) usage(description string) string {
	if s.EnvironmentName == "" {
		return description
	}

	return description + " (" + s.EnvironmentName + ")"
}
