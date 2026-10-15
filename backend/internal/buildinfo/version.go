// Package buildinfo provides release metadata injected at build time.
package buildinfo

// Release values default to development metadata for ordinary Go builds.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)
