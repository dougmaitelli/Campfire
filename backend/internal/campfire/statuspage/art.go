package statuspage

import (
	"net/url"
	"path"
	"strings"

	"campfire/internal/shared/model"
)

// PublicArtURL excludes private endpoint paths and credentials from published art.
func PublicArtURL(value string) string {
	// Browsers treat backslashes as separators in HTTPS URLs, unlike net/url
	// and path.Clean. Reject ambiguous syntax before checking private routes.
	if value == "" || strings.Contains(value, "\\") || model.ValidateArtURL(value) != nil {
		return ""
	}

	parsed, err := url.Parse(value)
	if err != nil || strings.Contains(parsed.Path, "\\") || (parsed.Path != "" && path.Clean(parsed.Path) != parsed.Path) {
		return ""
	}

	for _, reserved := range []string{"/api", "/spark", "/downloads", "/__test"} {
		if parsed.Path == reserved || strings.HasPrefix(parsed.Path, reserved+"/") {
			return ""
		}
	}

	return value
}
