package cli

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
)

const defaultAgentListen = "0.0.0.0:8443"

// agentPublicAddress separates the reachable HTTPS address from the bind address.
func agentPublicAddress(listen, advertised, panel string) (string, error) {
	if listen == "" {
		return "", nil
	}

	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", errors.New("agent listener must be a host:port address")
	}

	if number, err := strconv.Atoi(port); err != nil || number < 0 || number > 65535 {
		return "", errors.New("agent listener port must be between 0 and 65535")
	}

	if advertised != "" {
		target, err := url.Parse(advertised)
		if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil || target.Path != "" && target.Path != "/" || target.RawQuery != "" || target.Fragment != "" || target.Opaque != "" {
			return "", errors.New("agent public URL must be an HTTPS origin without credentials, path, query or fragment")
		}

		if ip, err := netip.ParseAddr(target.Hostname()); err == nil && (ip.IsUnspecified() || ip.Zone() != "") {
			return "", errors.New("agent public URL must use a reachable hostname or IP address")
		}

		if target.Port() != "" {
			if number, err := strconv.Atoi(target.Port()); err != nil || number < 1 || number > 65535 {
				return "", errors.New("agent public URL port must be between 1 and 65535")
			}
		}

		target.Path = ""

		return target.String(), nil
	}

	if panel != "" {
		target, err := url.Parse(panel)
		if err != nil || target.Hostname() == "" {
			return "", errors.New("cannot derive agent hostname from panel public URL")
		}

		host = target.Hostname()
	}

	if ip, err := netip.ParseAddr(host); host == "" || err == nil && ip.IsUnspecified() {
		host = "localhost"
	}

	return "https://" + net.JoinHostPort(host, port), nil
}

func agentCertificateHosts(address string) []string {
	target, _ := url.Parse(address)

	hosts := []string{target.Hostname()}
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		if host != target.Hostname() {
			hosts = append(hosts, host)
		}
	}

	return hosts
}
