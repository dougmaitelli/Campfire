package runtime

import (
	"encoding/json"
	"errors"
)

// Count shared replacement values at every output location before marshaling.
func boundedConfigSize(value any, depth int, remaining *int) error {
	if depth > 64 || *remaining < 0 {
		return errors.New("rewritten configuration exceeds size or nesting limits")
	}

	*remaining -= 2

	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			*remaining -= len(key) + 4
			if err := boundedConfigSize(child, depth+1, remaining); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range item {
			if err := boundedConfigSize(child, depth+1, remaining); err != nil {
				return err
			}
		}
	case string:
		*remaining -= len(item)
	case json.Number:
		*remaining -= len(item)
	default:
		*remaining -= 5
	}

	if *remaining < 0 {
		return errors.New("rewritten configuration exceeds 1 MiB")
	}

	return nil
}
