package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func (s *ConfigurationStore) streamConsole(ctx context.Context, id, server string, emit func(model.ConsoleEvent) error) error {
	if !configurationID.MatchString(id) || !configurationID.MatchString(server) {
		return errors.New("invalid server identity")
	}

	data, err := os.ReadFile(filepath.Join(s.Directory, id, server+".json"))
	if err != nil {
		return errors.New("deliver server configuration first")
	}

	var d model.Deployment
	if err := json.Unmarshal(data, &d); err != nil || d.ServerID != server || d.Configuration.SparkID != id {
		return errors.New("invalid console placement")
	}

	runtime, ok := s.Runtime.(interface {
		StreamConsole(context.Context, model.Deployment, func(string) error) error
	})
	if !ok {
		return errors.New("console streaming unavailable")
	}

	if err := emit(model.ConsoleEvent{Ready: true}); err != nil {
		return err
	}

	return runtime.StreamConsole(ctx, d, func(text string) error {
		text = strings.ToValidUTF8(text, "\uFFFD")

		for len(text) > 0 {
			n := min(len(text), 8192)
			for n > 0 && n < len(text) && !utf8.RuneStart(text[n]) {
				n--
			}

			if err := emit(model.ConsoleEvent{Content: text[:n]}); err != nil {
				return err
			}

			text = text[n:]
		}

		return nil
	})
}

type consoleStreams struct {
	mu     sync.Mutex
	active map[string]*consoleProducer
	wg     sync.WaitGroup
}

type consoleProducer struct {
	cancel  context.CancelFunc
	credits chan struct{}
}

func (s *consoleStreams) close() {
	s.mu.Lock()
	for _, producer := range s.active {
		producer.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *consoleStreams) stop(id string) {
	s.mu.Lock()
	if producer := s.active[id]; producer != nil {
		producer.cancel()
	}
	s.mu.Unlock()
}

func (s *consoleStreams) acknowledge(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if producer := s.active[id]; producer != nil {
		select {
		case producer.credits <- struct{}{}:
		default:
		}
	}
}

func (s *consoleStreams) open(ctx context.Context, node string, frame protocol.ConsoleFrame, store *ConfigurationStore, send func(protocol.Message) error) error {
	if frame.ID == "" || len(frame.ID) > 128 || !configurationID.MatchString(frame.ServerID) || frame.Event != (model.ConsoleEvent{}) {
		return errors.New("invalid console open")
	}

	s.mu.Lock()
	if s.active == nil {
		s.active = make(map[string]*consoleProducer)
	}

	if s.active[frame.ID] != nil {
		s.mu.Unlock()
		return errors.New("duplicate console stream")
	}

	if len(s.active) >= 16 {
		s.mu.Unlock()

		frame.Event = model.ConsoleEvent{End: true, Error: "Spark console stream limit reached."}

		return send(protocol.Message{Type: "console", Protocol: protocol.ProtocolVersion, Console: &frame})
	}

	ctx, cancel := context.WithCancel(ctx)

	producer := &consoleProducer{cancel: cancel, credits: make(chan struct{}, 8)}
	for range cap(producer.credits) {
		producer.credits <- struct{}{}
	}

	s.active[frame.ID] = producer
	s.wg.Add(1)

	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer cancel()
		defer func() { s.mu.Lock(); delete(s.active, frame.ID); s.mu.Unlock() }()

		emit := func(event model.ConsoleEvent) error {
			// Per-stream credits pause Docker reads without blocking the session reader.
			select {
			case <-producer.credits:
			case <-ctx.Done():
				return ctx.Err()
			}

			packet := frame
			packet.Event = event

			return send(protocol.Message{Type: "console", Protocol: protocol.ProtocolVersion, Console: &packet})
		}

		err := store.streamConsole(ctx, node, frame.ServerID, emit)
		if ctx.Err() == nil {
			event := model.ConsoleEvent{End: true}
			if err != nil {
				event.Error = "Console unavailable or interrupted. Reconnecting…"
			}

			_ = emit(event)
		}
	}()

	return nil
}
