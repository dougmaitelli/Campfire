package spark

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// AgentConfig controls first enrollment and persistent agent state.
type AgentConfig struct {
	PanelURL    string
	DataDir     string
	CAFile      string
	Token       string
	dialContext func(context.Context, string, string) (net.Conn, error)
}
type agentIdentity struct {
	ID          string
	PanelURL    string
	Certificate []byte
	Key         []byte
	ServerCA    []byte
}

// RunAgent enrolls if necessary, then maintains an authenticated outbound connection.
func RunAgent(ctx context.Context, config AgentConfig) error {
	if config.DataDir == "" {
		return errors.New("spark data directory is required")
	}

	path := filepath.Join(config.DataDir, "identity.json")

	var identity agentIdentity

	data, err := os.ReadFile(path)
	if err == nil {
		if err := privatePermissions(path); err != nil {
			return err
		}

		if err := json.Unmarshal(data, &identity); err != nil {
			return fmt.Errorf("read agent identity: %w", err)
		}

		if config.PanelURL != "" && strings.TrimRight(config.PanelURL, "/") != identity.PanelURL {
			return errors.New("panel URL differs from the saved identity")
		}

		if config.Token != "" {
			return errors.New("an identity already exists; use a new data directory to re-enroll")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		identity, err = enrollAgent(ctx, config)
		if err != nil {
			return err
		}

		data, err = json.Marshal(identity)
		if err != nil {
			return err
		}

		if err := savePrivate(path, data); err != nil {
			return fmt.Errorf("save agent identity: %w", err)
		}

		slog.Info("Spark enrolled", "spark", identity.ID)
	} else {
		return err
	}

	target, err := panelTarget(identity.PanelURL)
	if err != nil {
		return err
	}

	tlsConfig, err := identityTLS(identity)
	if err != nil {
		return err
	}

	target.Scheme = "wss"
	target.Path = "/spark/connect"
	delay := time.Second

agentLoop:
	for ctx.Err() == nil {
		started := time.Now()

		err := connectAgent(ctx, target.String(), identity.ID, tlsConfig, config.dialContext, &ConfigurationStore{Directory: filepath.Join(config.DataDir, "servers")})
		if ctx.Err() != nil {
			break
		}

		slog.Warn("Spark disconnected; retrying", "error", err, "delay", delay)

		if time.Since(started) > time.Minute {
			delay = time.Second
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			break agentLoop
		case <-timer.C:
		}

		delay = min(delay*2, 30*time.Second)
	}

	return nil //nolint:nilerr // Context cancellation is a successful agent shutdown.
}

func panelTarget(address string) (*url.URL, error) {
	target, err := url.Parse(strings.TrimRight(address, "/"))
	if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" || target.Path != "" {
		return nil, errors.New("panel must be an HTTPS origin, e.g. https://panel.example.com:8443")
	}

	return target, nil
}

func serverTLS(ca []byte) (*tls.Config, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS13}

	if len(ca) > 0 {
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}

		if !pool.AppendCertsFromPEM(ca) {
			return nil, errors.New("invalid panel CA file")
		}

		config.RootCAs = pool
	}

	return config, nil
}

func identityTLS(identity agentIdentity) (*tls.Config, error) {
	config, err := serverTLS(identity.ServerCA)
	if err != nil {
		return nil, err
	}

	certificate, err := tls.X509KeyPair(identity.Certificate, identity.Key)
	if err != nil {
		return nil, fmt.Errorf("read identity certificate: %w", err)
	}

	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, err
	}

	if leaf.Subject.CommonName != identity.ID || identity.ID == "" {
		return nil, errors.New("identity certificate does not match Spark ID")
	}

	if time.Now().After(leaf.NotAfter) {
		return nil, errors.New("spark certificate expired; generate a new enrollment token and enroll into a new data directory")
	}

	config.Certificates = []tls.Certificate{certificate}

	return config, nil
}

func enrollAgent(ctx context.Context, config AgentConfig) (agentIdentity, error) {
	target, err := panelTarget(config.PanelURL)
	if err != nil {
		return agentIdentity{}, err
	}

	if config.Token == "" {
		return agentIdentity{}, errors.New("first enrollment requires CAMPFIRE_ENROLLMENT_TOKEN or --token-file")
	}

	var ca []byte
	if config.CAFile != "" {
		ca, err = os.ReadFile(config.CAFile)
		if err != nil {
			return agentIdentity{}, err
		}
	}

	tlsConfig, err := serverTLS(ca)
	if err != nil {
		return agentIdentity{}, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return agentIdentity{}, err
	}

	keyPEM, err := marshalKey(key)
	if err != nil {
		return agentIdentity{}, err
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "Spark enrollment"}}, key)
	if err != nil {
		return agentIdentity{}, err
	}

	body, err := json.Marshal(struct {
		Token string `json:"token"`
		CSR   []byte `json:"csr"`
	}{config.Token, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})})
	if err != nil {
		return agentIdentity{}, err
	}

	target.Path = "/spark/enroll"

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return agentIdentity{}, err
	}

	request.Header.Set("Content-Type", "application/json")

	transport := &http.Transport{TLSClientConfig: tlsConfig, DialContext: config.dialContext}
	defer transport.CloseIdleConnections()

	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	response, err := client.Do(request)
	if err != nil {
		return agentIdentity{}, fmt.Errorf("enrollment request: %w", err)
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return agentIdentity{}, fmt.Errorf("enrollment returned HTTP %d", response.StatusCode)
	}

	var enrollment Enrollment
	if err := json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&enrollment); err != nil {
		return agentIdentity{}, err
	}

	identity := agentIdentity{ID: enrollment.ID, PanelURL: strings.TrimRight(config.PanelURL, "/"), Certificate: enrollment.Certificate, Key: keyPEM, ServerCA: ca}
	if _, err := identityTLS(identity); err != nil {
		return agentIdentity{}, err
	}

	return identity, nil
}

func connectAgent(ctx context.Context, address, id string, config *tls.Config, netDial func(context.Context, string, string) (net.Conn, error), configurations *ConfigurationStore) error {
	dialer := websocket.Dialer{TLSClientConfig: config, HandshakeTimeout: 10 * time.Second, NetDialContext: netDial}

	conn, response, err := dialer.DialContext(ctx, address, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}

	if err != nil {
		return err
	}

	defer func() { _ = conn.Close() }()

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	// The read loop owns writes, so application messages have a single writer.
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}

	if err := conn.WriteJSON(Message{Type: "hello", Protocol: ProtocolVersion, SparkID: id, Hostname: hostname, Version: "dev"}); err != nil {
		return err
	}

	conn.SetReadLimit(2 << 20)

	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}

	var welcome Message
	if err := conn.ReadJSON(&welcome); err != nil {
		return err
	}

	if welcome.Type != "welcome" || welcome.Protocol != ProtocolVersion {
		return errors.New("unsupported Spark protocol")
	}

	slog.Info("Spark connected", "spark", id)

	for {
		if err := conn.SetReadDeadline(time.Now().Add(45 * time.Second)); err != nil {
			return err
		}

		var message Message
		if err := conn.ReadJSON(&message); err != nil {
			return err
		}

		if message.Protocol != ProtocolVersion {
			return errors.New("unsupported panel message")
		}

		outgoing := Message{Type: "heartbeat", Protocol: ProtocolVersion}
		switch message.Type {
		case "heartbeat":
		case "command":
			if message.Command == nil {
				return errors.New("missing command")
			}

			response := configurations.execute(ctx, id, *message.Command)
			outgoing = Message{Type: "response", Protocol: ProtocolVersion, Response: &response}
		default:
			return errors.New("unsupported panel message")
		}

		if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}

		if err := conn.WriteJSON(outgoing); err != nil {
			return err
		}
	}
}
