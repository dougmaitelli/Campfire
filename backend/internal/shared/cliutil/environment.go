// Package cliutil provides shared command-line environment and version helpers.
package cliutil

import (
	"fmt"
	"os"

	"campfire/internal/shared/buildinfo"
)

// Environment returns an explicit environment value or the default.
func Environment(name, fallback string) string {
	if value, exists := os.LookupEnv(name); exists {
		return value
	}

	return fallback
}

// PrintVersion writes the release metadata without initializing application state.
func PrintVersion(name string) error {
	_, err := fmt.Fprintf(os.Stdout, "%s %s (commit %s, built %s)\n", name, buildinfo.Version, buildinfo.Commit, buildinfo.Date)
	return err
}
