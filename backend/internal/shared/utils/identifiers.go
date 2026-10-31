// Package utils provides shared validation helpers.
package utils

// IsValidIdentifier accepts nonempty identifiers containing lowercase ASCII
// letters, digits, dots, hyphens, and underscores.
func IsValidIdentifier(value string) bool {
	if value == "" {
		return false
	}

	for _, r := range value {
		if !IsValidIdentifierCharacter(r) {
			return false
		}
	}

	return true
}

// IsValidIdentifierCharacter reports whether a rune is allowed in an identifier.
func IsValidIdentifierCharacter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_'
}

// IsValidResourceID accepts 1–128 ASCII letters, digits, hyphens, or underscores.
func IsValidResourceID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}

	for _, r := range value {
		if !isASCIIAlphanumeric(r) && r != '-' && r != '_' {
			return false
		}
	}

	return true
}

// IsValidUsername accepts 3–64 lowercase identifier characters, starting with
// an ASCII letter or digit.
func IsValidUsername(value string) bool {
	return len(value) >= 3 && len(value) <= 64 && IsValidIdentifier(value) && isASCIIAlphanumeric(rune(value[0]))
}

func isASCIIAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
