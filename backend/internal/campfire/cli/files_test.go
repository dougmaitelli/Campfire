package cli

import "testing"

func TestParseMaxUpload(t *testing.T) {
	for _, raw := range []string{"0", "-1", "102401", "1.5", "invalid", "9223372036854775807"} {
		if _, err := parseMaxUpload(raw); err == nil {
			t.Errorf("accepted invalid upload limit %q", raw)
		}
	}

	for _, value := range []struct {
		raw   string
		bytes int64
	}{{"1", 1 << 20}, {"1024", 1 << 30}, {"102400", 100 << 30}} {
		actual, err := parseMaxUpload(value.raw)
		if err != nil || actual != value.bytes {
			t.Fatalf("parse %s: %d %v", value.raw, actual, err)
		}
	}
}
