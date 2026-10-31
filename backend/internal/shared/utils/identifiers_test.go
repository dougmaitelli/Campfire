package utils

import (
	"regexp"
	"strings"
	"testing"
)

func TestIdentifierValidationPreservesExistingRules(t *testing.T) {
	rules := []struct {
		name     string
		validate func(string) bool
		previous *regexp.Regexp
	}{
		{"identifier", IsValidIdentifier, regexp.MustCompile(`^[a-z0-9_.-]+$`)},
		{"resource ID", IsValidResourceID, regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)},
		{"username", IsValidUsername, regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,63}$`)},
	}

	values := []string{"", ".", "_", "-", "server.configure.v2", "TEST-server_12", "a/b", "é", "日", "a\xffb", "a\u200bb"}
	for r := rune(0); r < 128; r++ {
		values = append(values, string(r), "a"+string(r)+"z", string(r)+"az", "az"+string(r))
	}

	for _, length := range []int{1, 2, 3, 63, 64, 65, 127, 128, 129} {
		values = append(values, strings.Repeat("a", length), strings.Repeat("A", length), strings.Repeat("_", length))
	}

	for _, rule := range rules {
		t.Run(rule.name, func(t *testing.T) {
			for _, value := range values {
				if got, want := rule.validate(value), rule.previous.MatchString(value); got != want {
					t.Errorf("validation changed for %q: got %v, want %v", value, got, want)
				}
			}
		})
	}
}

func TestHostnameValidationBoundaries(t *testing.T) {
	maximum := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)
	for _, test := range []struct {
		value string
		valid bool
	}{
		{"localhost", true},
		{"Example.COM.", true},
		{"a-b.example", true},
		{"123", true},
		{strings.Repeat("a", 63), true},
		{maximum, true},
		{maximum + ".", true},
		{"", false},
		{".", false},
		{".example", false},
		{"example..", false},
		{"a..example", false},
		{"-example", false},
		{"example-", false},
		{"a_b.example", false},
		{"é.example", false},
		{"example\n", false},
		{"example\x00", false},
		{"example\xff", false},
		{"https://example.com", false},
		{"example.com:8080", false},
		{"[::1]", false},
		{strings.Repeat("a", 64), false},
		{maximum + "a", false},
	} {
		if got := IsValidHostname(test.value); got != test.valid {
			t.Errorf("hostname %q: got %v, want %v", test.value, got, test.valid)
		}
	}
}
