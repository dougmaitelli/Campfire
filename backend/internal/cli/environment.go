package cli

import "campfire/internal/cliutil"

func environment(name, fallback string) string { return cliutil.Environment(name, fallback) }

func printVersion(name string) error { return cliutil.PrintVersion(name) }
