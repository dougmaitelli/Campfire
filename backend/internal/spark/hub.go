package spark

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ProtocolVersion is the supported panel-agent wire protocol.
const ProtocolVersion = 1

// EnrollmentStore is the persistence boundary for agent authentication.
type EnrollmentStore interface {
	CreateToken(context.Context, string, string, time.Time) error
	Redeem(context.Context, string, func(string) (string, error)) (string, error)
	Authorized(context.Context, string, string) bool
}

// EnrollmentToken is returned once to the administrator.
type EnrollmentToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Enrollment returns a signed identity after consuming a bootstrap token.
type Enrollment struct {
	ID          string `json:"id"`
	Certificate []byte `json:"certificate"`
	CA          []byte `json:"ca"`
}

// Message carries versioned agent handshake and heartbeat data.
type Message struct {
	Type     string `json:"type"`
	Protocol int    `json:"protocol"`
	SparkID  string `json:"spark_id,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Version  string `json:"version,omitempty"`
}

// AgentInfo describes a currently connected agent.
type AgentInfo struct {
	Hostname string
	Version  string
}
type peer struct {
	conn *websocket.Conn
	info AgentInfo
}

// Hub owns live remote-agent connections; connection state is never persisted.
type Hub struct {
	store     EnrollmentStore
	authority *Authority
	mu        sync.RWMutex
	peers     map[string]*peer
	closed    bool
}

// NewHub creates an agent connection manager.
func NewHub(store EnrollmentStore, authority *Authority) *Hub {
	return &Hub{store: store, authority: authority, peers: make(map[string]*peer)}
}

// CreateEnrollment creates a single-use token that expires after fifteen minutes.
func (h *Hub) CreateEnrollment(ctx context.Context, id string) (EnrollmentToken, error) {
	result := EnrollmentToken{Token: rand.Text() + rand.Text(), ExpiresAt: time.Now().UTC().Add(15 * time.Minute)}
	if err := h.store.CreateToken(ctx, id, fingerprint([]byte(result.Token)), result.ExpiresAt); err != nil {
		return EnrollmentToken{}, err
	}

	return result, nil
}

// TLSConfig verifies agent certificates when supplied; enrollment itself uses a token.
func (h *Hub) TLSConfig() *tls.Config {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(h.authority.PEM)

	return &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.VerifyClientCertIfGiven, ClientCAs: pool}
}

// Handler exposes bootstrap enrollment and authenticated agent connections.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /spark/enroll", h.enroll)
	mux.HandleFunc("GET /spark/connect", h.connect)

	return mux
}

func (h *Hub) enroll(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil {
		http.Error(w, "enrollment requires HTTPS", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16384)

	var input struct {
		Token string `json:"token"`
		CSR   []byte `json:"csr"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil || len(input.Token) < 32 || len(input.Token) > 256 {
		http.Error(w, "invalid enrollment", http.StatusBadRequest)
		return
	}

	var certificate []byte

	id, err := h.store.Redeem(r.Context(), fingerprint([]byte(input.Token)), func(id string) (string, error) {
		signed, err := h.authority.Sign(id, input.CSR)
		if err != nil {
			return "", err
		}

		certificate = signed
		block, _ := pem.Decode(signed)

		return fingerprint(block.Bytes), nil
	})
	if err != nil {
		http.Error(w, "invalid, expired, or ineligible enrollment", http.StatusUnauthorized)
		return
	}

	h.Disconnect(id)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	if err := json.NewEncoder(w).Encode(Enrollment{ID: id, Certificate: certificate, CA: h.authority.PEM}); err != nil {
		return
	}
}

func (h *Hub) connect(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, "verified agent certificate required", http.StatusUnauthorized)
		return
	}

	leaf := r.TLS.PeerCertificates[0]

	id, hash := leaf.Subject.CommonName, fingerprint(leaf.Raw)
	if !h.store.Authorized(r.Context(), id, hash) {
		http.Error(w, "unknown or disabled Spark", http.StatusUnauthorized)
		return
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "" }}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	defer func() { _ = conn.Close() }()

	conn.SetReadLimit(16384)

	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return
	}

	var hello Message
	if err := conn.ReadJSON(&hello); err != nil || hello.Type != "hello" || hello.Protocol != ProtocolVersion || hello.SparkID != id || len(hello.Hostname) > 253 || len(hello.Version) > 128 {
		return
	}

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return
	}

	if err := conn.WriteJSON(Message{Type: "welcome", Protocol: ProtocolVersion}); err != nil {
		return
	}

	current := &peer{conn: conn, info: AgentInfo{Hostname: hello.Hostname, Version: hello.Version}}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}

	previous := h.peers[id]

	h.peers[id] = current
	h.mu.Unlock()

	if previous != nil {
		_ = previous.conn.Close()
	}

	defer func() {
		h.mu.Lock()
		if h.peers[id] == current {
			delete(h.peers, id)
		}
		h.mu.Unlock()
	}()

	if !h.store.Authorized(r.Context(), id, hash) {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !h.store.Authorized(ctx, id, hash) {
					_ = conn.Close()
					return
				}

				if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
					_ = conn.Close()
					return
				}

				if err := conn.WriteJSON(Message{Type: "heartbeat", Protocol: ProtocolVersion}); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()

	for {
		if err := conn.SetReadDeadline(time.Now().Add(45 * time.Second)); err != nil {
			return
		}

		var message Message
		if err := conn.ReadJSON(&message); err != nil || message.Type != "heartbeat" || message.Protocol != ProtocolVersion {
			return
		}
	}
}

// Connected returns live connection state.
func (h *Hub) Connected(id string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.peers[id] != nil
}

// Info returns metadata from the current authenticated handshake.
func (h *Hub) Info(id string) AgentInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if current := h.peers[id]; current != nil {
		return current.info
	}

	return AgentInfo{}
}

// Disconnect closes a node's current session immediately.
func (h *Hub) Disconnect(id string) {
	h.mu.Lock()
	current := h.peers[id]
	delete(h.peers, id)
	h.mu.Unlock()

	if current != nil {
		_ = current.conn.Close()
	}
}

// Close terminates all remote sessions and prevents new sessions.
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	peers := h.peers
	h.peers = make(map[string]*peer)
	h.mu.Unlock()

	for _, current := range peers {
		_ = current.conn.Close()
	}
}
