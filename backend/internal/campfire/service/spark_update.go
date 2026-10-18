package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"campfire/internal/shared/protocol"
	"campfire/internal/shared/sparkrelease"
)

// SparkReleaseCheckInterval bounds GitHub checks independently of UI polling.
const SparkReleaseCheckInterval = time.Hour
const sparkReleaseCheckTimeout = 20 * time.Second

// SparkUpdater delivers an explicit client upgrade over an authenticated connection.
type SparkUpdater interface {
	UpdateSpark(context.Context, string, string) error
}

// SparkReleaseStatus is the last hourly release check; failures retain the last good version.
type SparkReleaseStatus struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
	Error         string `json:"error,omitempty"`
}

// ErrSparkUpdate describes a stale, unsupported or unnecessary upgrade request.
var ErrSparkUpdate = errors.New("spark must be connected, support remote updates, and be older than the selected latest release")

// SparkUpdateService caches release metadata once per hour for the whole panel.
type SparkUpdateService struct {
	sparks  *SparkService
	updater SparkUpdater
	fetch   func(context.Context) (string, error)
	mu      sync.RWMutex
	status  SparkReleaseStatus
	active  map[string]bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewSparkUpdateService creates the shared release checker.
func NewSparkUpdateService(sparks *SparkService, updater SparkUpdater) *SparkUpdateService {
	return &SparkUpdateService{sparks: sparks, updater: updater, fetch: sparkrelease.NewClient().Latest, active: make(map[string]bool)}
}

// WithFetcher supplies a deterministic release source for mock development.
func (s *SparkUpdateService) WithFetcher(fetch func(context.Context) (string, error)) *SparkUpdateService {
	s.fetch = fetch
	return s
}

// Status reads cached metadata without making a network request.
func (s *SparkUpdateService) Status() SparkReleaseStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

func (s *SparkUpdateService) check(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, sparkReleaseCheckTimeout)
	defer cancel()

	version, err := s.fetch(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status.CheckedAt = time.Now().Unix()

	if err != nil {
		slog.Warn("Could not check the latest Spark release", "error", err)

		s.status.Error = "Unable to check GitHub for the latest Spark release."

		return
	}

	if !sparkrelease.StableVersion(version) {
		s.status.Error = "The release source returned an invalid Spark version."
		return
	}

	s.status.LatestVersion, s.status.Error = version, ""
}

// Start checks at startup and hourly thereafter, independent of page visits.
func (s *SparkUpdateService) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Go(func() {
		s.check(ctx)

		ticker := time.NewTicker(SparkReleaseCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.check(ctx)
			}
		}
	})
}

// Close cancels in-flight checks before shutdown.
func (s *SparkUpdateService) Close() {
	if s.cancel != nil {
		s.cancel()
	}

	s.wg.Wait()
}

// Update revalidates the connected client's version and negotiated capability.
func (s *SparkUpdateService) Update(ctx context.Context, id, version string) error {
	s.mu.Lock()
	if s.active[id] {
		s.mu.Unlock()
		return ErrSparkUpdate
	}

	s.active[id] = true

	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.active, id); s.mu.Unlock() }()

	spark, err := s.sparks.Get(ctx, id)
	if err != nil {
		return err
	}

	if s.updater == nil || !spark.Connected || version != s.Status().LatestVersion || !sparkrelease.Newer(spark.AgentVersion, version) {
		return ErrSparkUpdate
	}

	if err := protocol.Require(spark.Capabilities, protocol.CapabilitySelfUpdate); err != nil {
		return err
	}

	return s.updater.UpdateSpark(ctx, id, version)
}
