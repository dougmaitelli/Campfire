package utils

import "strings"

// IsValidHostname accepts ASCII DNS names with an optional trailing root dot.
func IsValidHostname(host string) bool {
	host = strings.TrimSuffix(host, ".")
	if len(host) == 0 || len(host) > 253 {
		return false
	}

	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}

		for _, r := range label {
			if !isASCIIAlphanumeric(r) && r != '-' {
				return false
			}
		}
	}

	return true
}
