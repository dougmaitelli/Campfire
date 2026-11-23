package service

import (
	"strings"
	"testing"
)

func TestPasswordPolicy(t *testing.T) {
	for _, tc := range []struct {
		password string
		valid    bool
	}{
		{"A long password 1!", true},
		{"Äé١!" + strings.Repeat("😀", 11), true},
		{"a long password 1!", false},
		{"A LONG PASSWORD 1!", false},
		{"A long password !", false},
		{"A long password 1", false},
		{"Short1!", false},
		{"Aa1!" + strings.Repeat("😀", 256), false},
	} {
		if got := validAccountInput("admin", tc.password); got != tc.valid {
			t.Errorf("validAccountInput(%q) = %v, want %v", tc.password, got, tc.valid)
		}
	}
}
