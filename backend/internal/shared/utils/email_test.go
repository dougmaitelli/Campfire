package utils

import "testing"

func TestNormalizeEmailPreservesTheMailbox(t *testing.T) {
	for address, want := range map[string]string{
		" Élodie@EXAMPLE.test ": "Élodie@example.test",
		"Élodie@BÜCHER.example": "Élodie@xn--bcher-kva.example",
		"User@İ.example":        "User@xn--i-9bb.example",
		"User@[192.0.2.1]":      "User@[192.0.2.1]",
	} {
		if got := NormalizeEmail(address); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", address, got, want)
		}
	}
}
