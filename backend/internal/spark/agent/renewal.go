package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"campfire/internal/shared/agentidentity"
)

func renewalDue(identity agentIdentity) bool {
	block, _ := pem.Decode(identity.Certificate)
	if block == nil {
		return true
	}

	leaf, err := x509.ParseCertificate(block.Bytes)

	return err != nil || time.Until(leaf.NotAfter) <= 30*24*time.Hour
}

func renewAgent(ctx context.Context, identity agentIdentity, path string, dial func(context.Context, string, string) (net.Conn, error)) (agentIdentity, error) {
	config, err := identityTLS(identity)
	if err != nil {
		return identity, err
	}

	pair := config.Certificates[0]

	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, pair.PrivateKey)
	if err != nil {
		return identity, err
	}

	body, err := json.Marshal(struct {
		CSR []byte `json:"csr"`
	}{pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})})
	if err != nil {
		return identity, err
	}

	target, err := panelTarget(identity.PanelURL)
	if err != nil {
		return identity, err
	}

	target.Path = "/spark/renew"

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return identity, err
	}

	request.Header.Set("Content-Type", "application/json")

	transport := &http.Transport{TLSClientConfig: config, DialContext: dial}
	defer transport.CloseIdleConnections()

	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	response, err := client.Do(request)
	if err != nil {
		return identity, err
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return identity, fmt.Errorf("certificate renewal returned HTTP %d", response.StatusCode)
	}

	var result struct {
		Certificate []byte `json:"certificate"`
		ServerCA    []byte `json:"server_ca,omitempty"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&result); err != nil {
		return identity, err
	}

	updated := identity

	updated.Certificate = result.Certificate
	if len(result.ServerCA) > 0 {
		// Trust refresh arrives over the already-verified mutual TLS connection.
		// Retain normal hostname/chain checks before persisting its replacement.
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(result.ServerCA) || response.TLS == nil || len(response.TLS.PeerCertificates) == 0 {
			return identity, errors.New("invalid renewed agent HTTPS authority")
		}

		intermediates := x509.NewCertPool()
		for _, certificate := range response.TLS.PeerCertificates[1:] {
			intermediates.AddCert(certificate)
		}

		if _, err := response.TLS.PeerCertificates[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: target.Hostname()}); err != nil {
			return identity, fmt.Errorf("verify renewed agent HTTPS authority: %w", err)
		}

		updated.ServerCA = append([]byte(nil), result.ServerCA...)
	}

	newTLS, err := identityTLS(updated)
	if err != nil {
		return identity, err
	}

	oldLeaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return identity, err
	}

	newLeaf, err := x509.ParseCertificate(newTLS.Certificates[0].Certificate[0])
	if err != nil {
		return identity, err
	}

	if !newLeaf.NotAfter.After(oldLeaf.NotAfter) {
		return identity, errors.New("renewed certificate does not extend validity")
	}

	data, err := json.Marshal(updated)
	if err != nil {
		return identity, err
	}

	if err := agentidentity.WritePrivate(path, data); err != nil {
		return identity, fmt.Errorf("save renewed Spark identity: %w", err)
	}

	return updated, nil
}

func connectRenewing(ctx context.Context, address string, identity *agentIdentity, path string, dial func(context.Context, string, string) (net.Conn, error), store *ConfigurationStore) (bool, error) {
	if renewalDue(*identity) {
		updated, err := renewAgent(ctx, *identity, path, dial)
		if err == nil {
			*identity = updated
		} else {
			slog.Warn("Spark certificate renewal failed; retaining current identity", "error", err)
		}
	}

	config, err := identityTLS(*identity)
	if err != nil {
		return false, err
	}

	return maintainIdentity(ctx, address, identity, path, dial, store, config)
}

func maintainIdentity(ctx context.Context, address string, identity *agentIdentity, path string, dial func(context.Context, string, string) (net.Conn, error), store *ConfigurationStore, config *tls.Config) (bool, error) {
	session, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, 1)

	id := identity.ID
	go func() { done <- connectAgent(session, address, id, config, dial, store) }()

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			return false, err
		case <-ctx.Done():
			cancel()
			<-done

			return false, ctx.Err()
		case <-ticker.C:
			if !renewalDue(*identity) {
				continue
			}

			updated, err := renewAgent(ctx, *identity, path, dial)
			if err != nil {
				slog.Warn("Spark certificate renewal failed; retrying", "error", err)
				continue
			}

			cancel()
			<-done

			*identity = updated

			slog.Info("Spark certificate renewed", "spark", id)

			return true, nil
		}
	}
}
