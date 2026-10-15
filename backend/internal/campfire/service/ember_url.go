package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const maxEmberBytes = 1 << 20

// FetchEmberURL retrieves and validates a public HTTPS definition without saving it.
func FetchEmberURL(ctx context.Context, address string, options ImportOptions) (EmberPreview, error) {
	transport := &http.Transport{
		DialContext:           dialPublic,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport, Timeout: 10 * time.Second,
		CheckRedirect: func(r *http.Request, previous []*http.Request) error {
			if len(previous) >= 5 {
				return fmt.Errorf("%w: too many URL redirects", ErrInvalidEmber)
			}

			return validateEmberURL(r.URL)
		},
	}

	return fetchEmberURL(ctx, address, client, options)
}

func fetchEmberURL(ctx context.Context, address string, client *http.Client, options ImportOptions) (EmberPreview, error) {
	target, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		return EmberPreview{}, fmt.Errorf("%w: invalid definition URL", ErrInvalidEmber)
	}

	if err := validateEmberURL(target); err != nil {
		return EmberPreview{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return EmberPreview{}, fmt.Errorf("%w: invalid definition URL", ErrInvalidEmber)
	}

	request.Header.Set("Accept", "application/json, application/yaml, text/yaml, text/plain")

	response, err := client.Do(request)
	if err != nil {
		return EmberPreview{}, fmt.Errorf("%w: unable to fetch definition URL", ErrInvalidEmber)
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return EmberPreview{}, fmt.Errorf("%w: definition URL returned HTTP %d", ErrInvalidEmber, response.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, maxEmberBytes+1))
	if err != nil {
		return EmberPreview{}, fmt.Errorf("%w: unable to read definition", ErrInvalidEmber)
	}

	if len(data) > maxEmberBytes {
		return EmberPreview{}, fmt.Errorf("%w: definitions must be at most 1 MiB", ErrInvalidEmber)
	}

	return PreviewEmber(data, options), nil
}

func validateEmberURL(target *url.URL) error {
	if target.Scheme != "https" || target.Hostname() == "" || target.User != nil || target.Fragment != "" {
		return fmt.Errorf("%w: use a public HTTPS URL without credentials or a fragment", ErrInvalidEmber)
	}

	if ip, err := netip.ParseAddr(target.Hostname()); err == nil && !publicAddress(ip) {
		return fmt.Errorf("%w: URL must point to a public address", ErrInvalidEmber)
	}

	return nil
}

func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}

	for _, prefix := range []string{"100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "fec0::/10"} {
		if netip.MustParsePrefix(prefix).Contains(ip) {
			return false
		}
	}

	return true
}

func dialPublic(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}

	for _, ip := range addresses {
		if !publicAddress(ip) {
			return nil, fmt.Errorf("definition URL resolved to a non-public address")
		}
	}

	dialer := net.Dialer{Timeout: 5 * time.Second}
	for _, ip := range addresses {
		connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return connection, nil
		}
	}

	return nil, fmt.Errorf("unable to connect to definition host")
}
