package utils

import (
	"strings"

	"golang.org/x/net/idna"
)

// NormalizeEmail keeps the mailbox local-part intact. Only the domain is case-insensitive.
func NormalizeEmail(address string) string {
	address = strings.TrimSpace(address)

	at := strings.LastIndexByte(address, '@')
	if at < 0 {
		return address
	}

	domain, err := idna.Lookup.ToASCII(address[at+1:])
	if err != nil {
		// Non-DNS forms such as address literals retain their original spelling.
		return address
	}

	return address[:at+1] + domain
}
