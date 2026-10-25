package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

// Each read-only directory mount exposes exactly one server's endpoint. No game
// chooses its source identity and no host TCP listener is required.
type gameIntegrations struct {
	ctx        context.Context
	directory  string
	authorize  func(string) bool
	mu         sync.Mutex
	endpoints  map[string]*http.Server
	closed     bool
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	events     map[string]*queuedGameEvent
	wake       chan struct{}
	dropped    map[string]int
	failures   map[string]string
	session    *gameSession
	slots      chan struct{}
	mediaSlots chan struct{}
}

type gameSession struct {
	events  bool
	ctx     context.Context
	send    func(protocol.Message) error
	pending map[string]chan *protocol.CommandError
	upload  gameMediaSender
}

func newGameIntegrations(ctx context.Context, directory string, authorize func(string) bool) (*gameIntegrations, error) {
	directory, err := filepath.Abs(directory)
	if err != nil || strings.ContainsAny(directory, ",\x00") {
		return nil, errors.New("invalid game integration directory")
	}

	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}

	if err := os.Chmod(directory, 0700); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	g := &gameIntegrations{ctx: ctx, cancel: cancel, directory: directory, authorize: authorize, endpoints: make(map[string]*http.Server), slots: make(chan struct{}, 32), events: make(map[string]*queuedGameEvent), wake: make(chan struct{}, 1), dropped: make(map[string]int), failures: make(map[string]string)}

	g.mediaSlots = make(chan struct{}, 2)
	if err := g.loadEvents(); err != nil {
		cancel()
		return nil, err
	}

	g.wg.Add(1)
	go g.runEvents()

	return g, nil
}

func (g *gameIntegrations) mount(id string) (string, error) {
	if !configurationID.MatchString(id) || !g.authorize(id) {
		return "", errors.New("invalid game integration server")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed || g.ctx.Err() != nil {
		return "", errors.New("game integration is shutting down")
	}

	hash := sha256.Sum256([]byte(id))

	directory := filepath.Join(g.directory, hex.EncodeToString(hash[:16]))
	if g.endpoints[id] != nil {
		return directory, nil
	}

	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}

	if err := os.Chmod(directory, 0755); err != nil {
		return "", err
	}

	path := filepath.Join(directory, "publish.sock")
	// Only remove a stale socket, never an unexpected file or symlink.
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return "", errors.New("game endpoint path is not a socket")
		}

		if err := os.Remove(path); err != nil {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	listener, err := (&net.ListenConfig{}).Listen(g.ctx, "unix", path)
	if err != nil {
		return "", err
	}

	if err := os.Chmod(path, 0666); err != nil {
		_ = listener.Close()
		return "", err
	}
	// The private host parent and per-container read-only mount provide access
	// control. The socket supports game processes running under arbitrary UIDs.
	var (
		rateMu   sync.Mutex
		window   time.Time
		requests int
	)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v1/events/{event}/media/{attachment}", func(w http.ResponseWriter, r *http.Request) { g.mediaUpload(w, r, id) })
	mux.HandleFunc("GET /v1/events/queue", func(w http.ResponseWriter, r *http.Request) { g.queueStatus(w, r, id) })
	mux.HandleFunc("POST /v1/publish", func(w http.ResponseWriter, r *http.Request) {
		rateMu.Lock()
		if time.Since(window) >= time.Second {
			window, requests = time.Now(), 0
		}

		requests++
		limited := requests > 10
		rateMu.Unlock()

		if limited {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "publication rate exceeded", http.StatusTooManyRequests)

			return
		}

		g.publish(w, r, id)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 12 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 4096}

	g.endpoints[id] = server
	go func() { _ = server.Serve(listener) }()

	return directory, nil
}

func (g *gameIntegrations) close() {
	g.mu.Lock()

	g.closed = true

	for _, server := range g.endpoints {
		_ = server.Close()
	}
	g.mu.Unlock()

	if g.cancel != nil {
		g.cancel()
	}

	g.wg.Wait()
}

func (g *gameIntegrations) publish(w http.ResponseWriter, r *http.Request, id string) {
	if !g.authorize(id) {
		http.Error(w, "server no longer assigned to this Spark", http.StatusForbidden)
		return
	}

	select {
	case g.slots <- struct{}{}:
		defer func() { <-g.slots }()
	default:
		http.Error(w, "integration busy", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, model.GamePublicationLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var publication model.GamePublication
	if err := decoder.Decode(&publication); err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			http.Error(w, "publication too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "invalid publication", http.StatusBadRequest)
		}

		return
	}

	err := decoder.Decode(new(any))

	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		http.Error(w, "publication too large", http.StatusRequestEntityTooLarge)
		return
	}

	if err != io.EOF || publication.Validate() != nil {
		http.Error(w, "invalid publication", http.StatusBadRequest)
		return
	}

	if publication.Event != nil {
		if err := g.enqueueEvent(id, publication); err != nil {
			status := http.StatusServiceUnavailable
			if errors.Is(err, errGameQueueFull) {
				status = http.StatusTooManyRequests

				w.Header().Set("Retry-After", "5")
			}

			if errors.Is(err, errGameEventConflict) {
				status = http.StatusConflict
			}

			http.Error(w, err.Error(), status)

			return
		}

		w.WriteHeader(http.StatusAccepted)

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	if err := g.forward(ctx, id, publication); err != nil {
		http.Error(w, "publication not acknowledged; retry with the same event ID", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (g *gameIntegrations) forward(ctx context.Context, serverID string, publication model.GamePublication) error {
	requestID := rand.Text()
	ack := make(chan *protocol.CommandError, 1)

	g.mu.Lock()

	session := g.session
	if session == nil || session.ctx.Err() != nil {
		g.mu.Unlock()
		return errors.New("game integration disconnected or unsupported")
	}

	if publication.Event != nil && !session.events {
		g.mu.Unlock()
		return errors.New("durable game events require an updated Campfire")
	}

	session.pending[requestID] = ack

	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(session.pending, requestID)
		g.mu.Unlock()
	}()

	if err := session.send(protocol.Message{Type: "game.publish", Game: &protocol.GameFrame{ID: requestID, ServerID: serverID, Publication: publication}}); err != nil {
		return err
	}

	select {
	case failure := <-ack:
		if failure != nil {
			return failure
		}

		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-session.ctx.Done():
		return session.ctx.Err()
	}
}

func (g *gameIntegrations) attach(ctx context.Context, send func(protocol.Message) error, durableEvents ...bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.session = &gameSession{ctx: ctx, send: send, pending: make(map[string]chan *protocol.CommandError), events: len(durableEvents) > 0 && durableEvents[0]}
	for _, item := range g.events {
		item.next = time.Time{}
	}

	select {
	case g.wake <- struct{}{}:
	default:
	}
}

func (g *gameIntegrations) acknowledge(ack *protocol.GameAck) error {
	if ack == nil || ack.ID == "" || len(ack.ID) > 128 {
		return errors.New("invalid game acknowledgement")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.session != nil {
		if pending := g.session.pending[ack.ID]; pending != nil {
			select {
			case pending <- ack.Error:
			default:
			}
		}
	}

	return nil
}

// savedGame checks local placement on every request; panel ingestion independently
// checks authoritative placement, including transfers away from this Spark.
func (s *ConfigurationStore) savedGame(sparkID, serverID string) bool {
	if !configurationID.MatchString(sparkID) || !configurationID.MatchString(serverID) {
		return false
	}

	data, err := os.ReadFile(filepath.Join(s.Directory, sparkID, serverID+".json"))
	if err != nil {
		return false
	}

	var deployment model.Deployment

	return json.Unmarshal(data, &deployment) == nil && deployment.ServerID == serverID && deployment.Configuration.SparkID == sparkID
}
