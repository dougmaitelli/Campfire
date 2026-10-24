// Package agent manages Spark's outbound connection and game execution.
package agent

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
	"sync"
	"time"

	"campfire/internal/shared/agentidentity"
	"campfire/internal/shared/buildinfo"
	"campfire/internal/shared/protocol"
	"campfire/internal/shared/sparkrelease"
	"campfire/internal/spark/runtime"

	"github.com/gorilla/websocket"
)

// Config controls first enrollment and persistent agent state.
type Config struct {
	PanelURL         string
	DataDir          string
	CAFile           string
	Token            string
	EnrollOnly       bool
	GameIntegrations bool
	dialContext      func(context.Context, string, string) (net.Conn, error)
	runtimeFactory   func(context.Context, string) *runtime.Manager
	serverCA         []byte
}

type agentIdentity struct {
	ID          string
	PanelURL    string
	Certificate []byte
	Key         []byte
	ServerCA    []byte
}

// Run enrolls if necessary, then maintains an authenticated outbound connection.
func Run(ctx context.Context, config Config) error {
	if config.DataDir == "" {
		return errors.New("spark data directory is required")
	}

	if !config.EnrollOnly {
		if err := runtime.ValidateDockerConfiguration(); err != nil {
			return err
		}
	}

	path := filepath.Join(config.DataDir, "identity.json")

	var identity agentIdentity

	data, err := os.ReadFile(path)
	if err == nil {
		if err := agentidentity.CheckPermissions(path); err != nil {
			return err
		}

		if err := json.Unmarshal(data, &identity); err != nil {
			return fmt.Errorf("read agent identity: %w", err)
		}

		if config.PanelURL != "" && strings.TrimRight(config.PanelURL, "/") != identity.PanelURL {
			return errors.New("panel URL differs from the saved identity")
		}

		if config.Token != "" {
			recovery := config
			recovery.PanelURL = identity.PanelURL
			recovery.serverCA = identity.ServerCA

			updated, err := enrollAgent(ctx, recovery)
			if err != nil {
				return fmt.Errorf("re-enroll Spark: %w", err)
			}

			if updated.ID != identity.ID {
				return errors.New("re-enrollment token must belong to this Spark; existing data and identity were preserved")
			}

			data, err := json.Marshal(updated)
			if err != nil {
				return err
			}

			if err := agentidentity.WritePrivate(path, data); err != nil {
				return err
			}

			identity = updated
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

		if err := agentidentity.WritePrivate(path, data); err != nil {
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

	if _, err := identityTLS(identity); err != nil {
		return err
	}

	if config.EnrollOnly {
		return nil
	}

	target.Scheme = "wss"
	target.Path = "/spark/connect"
	delay := time.Second

	factory := config.runtimeFactory
	if factory == nil {
		factory = runtime.New
	}

	manager := factory(ctx, filepath.Join(config.DataDir, "runtime"))
	if manager == nil {
		return errors.New("spark runtime is unavailable")
	}
	defer manager.Close()

	slog.Info("Game-data disk quota support", "enabled", manager.DiskQuotaSupported())

	configurations := &ConfigurationStore{Directory: filepath.Join(config.DataDir, "servers"), Runtime: manager}
	if config.GameIntegrations {
		sparkID := identity.ID

		games, err := newGameIntegrations(ctx, filepath.Join(config.DataDir, "game-integrations"), func(serverID string) bool { return configurations.savedGame(sparkID, serverID) })
		if err != nil {
			return err
		}
		defer games.close()

		configurations.games = games
		manager.GameIntegrationMount = games.mount
		// Recreate endpoints for Docker containers surviving a Spark restart.
		entries, err := os.ReadDir(filepath.Join(configurations.Directory, identity.ID))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}

			if _, err := games.mount(strings.TrimSuffix(entry.Name(), ".json")); err != nil {
				return fmt.Errorf("restore game integration endpoint: %w", err)
			}
		}
	}

	if config.Token == "" {
		configurations.Update = func(ctx context.Context, version string) (string, error) {
			tlsConfig, err := identityTLS(identity)
			if err != nil {
				return "", err
			}

			transport := &http.Transport{TLSClientConfig: tlsConfig, DialContext: config.dialContext}
			defer transport.CloseIdleConnections()

			client := sparkrelease.NewClient(identity.PanelURL+"/spark/downloads", transport)

			return installSparkUpdate(ctx, version, configurations.NegotiatedProtocol, manager.CanUpdate, client)
		}
	}

agentLoop:
	for ctx.Err() == nil {
		started := time.Now()

		renewed, err := connectRenewing(ctx, target.String(), &identity, path, config.dialContext, configurations)

		var restart *RestartError
		if errors.As(err, &restart) {
			return restart
		}

		if ctx.Err() != nil {
			break
		}

		if renewed {
			delay = time.Second
			continue
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
		return nil, errors.New("spark certificate expired; generate a new enrollment token for this Spark and run with --token-file using the same data directory")
	}

	if time.Now().Before(leaf.NotBefore) {
		return nil, errors.New("spark certificate is not yet valid; check host clocks")
	}

	config.Certificates = []tls.Certificate{certificate}

	return config, nil
}

func enrollAgent(ctx context.Context, config Config) (agentIdentity, error) {
	target, err := panelTarget(config.PanelURL)
	if err != nil {
		return agentIdentity{}, err
	}

	if config.Token == "" {
		return agentIdentity{}, errors.New("first enrollment requires CAMPFIRE_ENROLLMENT_TOKEN or --token-file")
	}

	ca := config.serverCA
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

	keyPEM, err := agentidentity.MarshalKey(key)
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

	var enrollment protocol.Enrollment
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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
	// Console producers and command replies share one serialized writer.
	var writeMu sync.Mutex

	wireVersion := protocol.ProtocolVersion

	send := func(message protocol.Message) error {
		message.Protocol = wireVersion

		writeMu.Lock()
		defer writeMu.Unlock()

		if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}

		if err := conn.WriteJSON(message); err != nil {
			cancel()

			_ = conn.Close()

			return err
		}

		return nil
	}

	streams := &consoleStreams{}
	defer streams.close()
	defer cancel()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}

	offered := configurations.capabilities()

	if err := conn.WriteJSON(protocol.Message{Type: "hello", Protocol: protocol.ProtocolVersion, MinimumProtocol: protocol.MinimumProtocolVersion, Capabilities: offered, SparkID: id, Hostname: hostname, Version: buildinfo.SparkVersion()}); err != nil {
		return err
	}

	conn.SetReadLimit(2 << 20)

	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}

	var welcome protocol.Message
	if err := conn.ReadJSON(&welcome); err != nil {
		return err
	}

	if err := protocol.ValidateWelcome(welcome, offered); err != nil {
		return err
	}

	wireVersion = welcome.Protocol

	configurations.NegotiatedProtocol = wireVersion
	if configurations.games != nil && protocol.Require(welcome.Capabilities, protocol.CapabilityGameIntegration) == nil {
		configurations.games.attach(ctx, send)
	}

	slog.Info("Spark connected", "spark", id, "protocol", wireVersion, "capabilities", welcome.Capabilities)

	for {
		if err := conn.SetReadDeadline(time.Now().Add(45 * time.Second)); err != nil {
			return err
		}

		var message protocol.Message
		if err := conn.ReadJSON(&message); err != nil {
			return err
		}

		if message.Protocol != wireVersion {
			return errors.New("unsupported panel message")
		}

		outgoing := protocol.Message{Type: "heartbeat"}
		switch message.Type {
		case "heartbeat":
		case "game.ack":
			if configurations.games == nil || protocol.Require(welcome.Capabilities, protocol.CapabilityGameIntegration) != nil {
				return errors.New("unnegotiated game acknowledgement")
			}

			if err := configurations.games.acknowledge(message.GameAck); err != nil {
				return err
			}

			continue
		case "console.open":
			if err := protocol.Require(welcome.Capabilities, protocol.CapabilityConsole); err != nil {
				return err
			}

			if message.Console == nil {
				return errors.New("missing console stream")
			}

			if err := streams.open(ctx, id, *message.Console, configurations, send); err != nil {
				return err
			}

			continue
		case "console.close":
			if err := protocol.Require(welcome.Capabilities, protocol.CapabilityConsole); err != nil {
				return err
			}

			if message.Console == nil {
				return errors.New("missing console stream")
			}

			streams.stop(message.Console.ID)

			continue
		case "console.ack":
			if err := protocol.Require(welcome.Capabilities, protocol.CapabilityConsole); err != nil {
				return err
			}

			if message.Console == nil {
				return errors.New("missing console acknowledgement")
			}

			streams.acknowledge(message.Console.ID)

			continue
		case "command":
			if message.Command == nil {
				return errors.New("missing command")
			}

			var response protocol.CommandResponse
			if message.Command.Name == protocol.UpdateSpark || message.Command.Name == protocol.PruneSpark {
				response = updateWithHeartbeats(ctx, func() protocol.CommandResponse {
					return configurations.executeNegotiated(ctx, id, *message.Command, welcome.Capabilities)
				}, send)
			} else {
				response = configurations.executeNegotiated(ctx, id, *message.Command, welcome.Capabilities)
			}

			outgoing = protocol.Message{Type: "response", Response: &response}
		default:
			return errors.New("unsupported panel message")
		}

		sendErr := send(outgoing)

		if configurations.RestartExecutable != "" {
			return &RestartError{Executable: configurations.RestartExecutable}
		}

		if err := sendErr; err != nil {
			return err
		}
	}
}
