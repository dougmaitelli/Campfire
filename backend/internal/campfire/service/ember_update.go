package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"campfire/internal/shared/model"
)

const emberCheckInterval = 24 * time.Hour

// EmberUpdateStore persists check results across panel restarts.
type EmberUpdateStore interface {
	List(context.Context) ([]model.EmberUpdate, error)
	Save(context.Context, model.EmberUpdate) error
}

// EmberUpdateService checks remote definitions without changing pinned revisions.
type EmberUpdateService struct {
	Embers *EmberService
	Store  EmberUpdateStore
	fetch  func(context.Context, string, ImportOptions) (EmberPreview, error)
	now    func() time.Time
	gate   chan struct{}
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewEmberUpdateService creates a daily update checker.
func NewEmberUpdateService(embers *EmberService, store EmberUpdateStore) *EmberUpdateService {
	return &EmberUpdateService{Embers: embers, Store: store, fetch: FetchEmberURL, now: time.Now, gate: make(chan struct{}, 1)}
}

// WithFetcher supplies a transport for isolated development fixtures.
func (s *EmberUpdateService) WithFetcher(fetch func(context.Context, string, ImportOptions) (EmberPreview, error)) *EmberUpdateService {
	s.fetch = fetch
	return s
}

func emberUpdateURL(ember model.Ember) string {
	if ember.UpdateURL != "" {
		return ember.UpdateURL
	}

	if ember.Origin != nil {
		return ember.Origin.UpdateURL
	}

	return ""
}

// fingerprint compares parsed definitions so formatting and object key order do not count as updates.
func emberFingerprint(ember model.Ember) [32]byte {
	ember.ID = ""
	// Attribution and update locations do not change the game's definition.
	ember.Origin = nil
	ember.UpdateURL = ""
	data, _ := json.Marshal(ember)

	return sha256.Sum256(data)
}

// Check refreshes one revision's status and returns the downloaded preview.
func (s *EmberUpdateService) Check(ctx context.Context, id string) (model.EmberUpdate, EmberPreview, error) {
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return model.EmberUpdate{}, EmberPreview{}, ctx.Err()
	}

	return s.check(ctx, id)
}

func (s *EmberUpdateService) check(ctx context.Context, id string) (model.EmberUpdate, EmberPreview, error) {
	ember, err := s.Embers.Get(ctx, id)
	if err != nil {
		return model.EmberUpdate{}, EmberPreview{}, err
	}

	address := emberUpdateURL(ember)
	if address == "" {
		return model.EmberUpdate{}, EmberPreview{}, fmt.Errorf("%w: this Ember has no update URL", ErrInvalidEmber)
	}

	preview, fetchErr := s.fetch(ctx, address, ImportOptions{})
	// Older Pelican revisions did not retain their startup selection. Match it
	// against the available commands before comparing the converted definition.
	if fetchErr == nil && preview.Ember == nil && len(preview.StartupCommands) > 0 {
		var first *EmberPreview

		for _, command := range preview.StartupCommands {
			candidate := PreviewEmber([]byte(preview.Definition), ImportOptions{SourceURL: address, StartupCommand: command})

			candidate.SelectedStartupCommand = command
			if candidate.Ember == nil {
				continue
			}

			if first == nil {
				first = &candidate
			}

			if candidate.Ember.Startup == ember.Startup {
				first = &candidate
				break
			}
		}

		if first != nil {
			preview = *first
		}
	}

	item := model.EmberUpdate{EmberID: id, URL: address, CheckedAt: s.now().UTC().Unix()}
	if fetchErr != nil {
		item.Error = fetchErr.Error()
	} else if preview.Ember == nil || len(preview.Errors) > 0 {
		item.Error = "The update URL did not return a compatible definition."
	} else {
		item.Available = emberFingerprint(ember) != emberFingerprint(*preview.Ember)
	}

	if err := s.Store.Save(ctx, item); err != nil {
		return item, preview, err
	}

	return item, preview, nil
}

// Sweep checks due revisions, including never-checked imports, on startup and every ten minutes.
func (s *EmberUpdateService) Sweep(ctx context.Context) error {
	items, err := s.Embers.List(ctx)
	if err != nil {
		return err
	}

	checks, err := s.Store.List(ctx)
	if err != nil {
		return err
	}

	previous := make(map[string]model.EmberUpdate, len(checks))
	for _, item := range checks {
		previous[item.EmberID] = item
	}

	for _, ember := range items {
		if err := ctx.Err(); err != nil {
			return err
		}

		address := emberUpdateURL(ember)

		last := previous[ember.ID]
		if address == "" || (last.URL == address && s.now().Sub(time.Unix(last.CheckedAt, 0)) < emberCheckInterval) {
			continue
		}

		if _, _, err := s.Check(ctx, ember.ID); err != nil {
			return err
		}
	}

	return nil
}

// Start runs due checks until the panel shuts down.
func (s *EmberUpdateService) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			if err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
				slog.Error("check Ember updates", "error", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// Close cancels pending downloads and waits for the worker.
func (s *EmberUpdateService) Close() {
	if s.cancel != nil {
		s.cancel()
	}

	s.wg.Wait()
}
