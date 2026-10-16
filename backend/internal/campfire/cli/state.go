package cli

import (
	"path/filepath"
	"strings"
)

func stateDirectory(source, configured string) string {
	if configured != "" {
		return configured
	}

	if strings.HasPrefix(source, "postgres://") || strings.HasPrefix(source, "postgresql://") {
		return "data"
	}

	return filepath.Dir(source)
}
