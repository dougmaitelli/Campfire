// Package gameaddress resolves display-only player connection addresses.
package gameaddress

import (
	"net"
	"net/netip"
	"strconv"
)

// Resolve never interprets an explicit server override or uses a bind interface.
func Resolve(override, host string, port int) string {
	if override != "" {
		return override
	}

	if !UsableDefaultHost(host) || port < 1 || port > 65535 {
		return ""
	}

	return net.JoinHostPort(host, strconv.Itoa(port))
}

// UsableDefaultHost checks a validated IP or hostname for an inherited address.
// Keep legacy unspecified/multicast defaults unpublished without constraining
// deliberately free-form server overrides.
func UsableDefaultHost(host string) bool {
	if host == "" {
		return false
	}

	ip, err := netip.ParseAddr(host)
	if err != nil {
		return true
	}

	ip = ip.Unmap()

	return !ip.IsUnspecified() && !ip.IsMulticast()
}
