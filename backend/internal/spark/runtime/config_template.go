package runtime

import (
	"errors"
	"strings"
)

// renderBounded substitutes once; variable contents never become new templates.
func renderBounded(value string, env map[string]string, limit int) (string, error) {
	var output strings.Builder

	appendText := func(text string) error {
		if len(text) > limit-output.Len() {
			return errors.New("expanded template exceeds its size limit")
		}

		_, err := output.WriteString(text)

		return err
	}

	for value != "" {
		start := strings.Index(value, "{{")
		if start < 0 {
			if err := appendText(value); err != nil {
				return "", err
			}

			break
		}

		end := strings.Index(value[start+2:], "}}")
		if end < 0 {
			if err := appendText(value); err != nil {
				return "", err
			}

			break
		}

		end += start + 2
		if err := appendText(value[:start]); err != nil {
			return "", err
		}

		text := value[start : end+2]
		if replacement, ok := env[value[start+2:end]]; ok {
			text = replacement
		}

		if err := appendText(text); err != nil {
			return "", err
		}

		value = value[end+2:]
	}

	return output.String(), nil
}
