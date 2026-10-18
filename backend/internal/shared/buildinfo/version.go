// Package buildinfo provides release metadata injected at build time.
package buildinfo

// Release values default to development metadata for ordinary Go builds.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// SparkVersion identifies development binaries by commit, matching their panel.
func SparkVersion() string {
	if Version == "dev" && Commit != "unknown" && Commit != "" {
		return "dev-" + Commit
	}

	return Version
}
