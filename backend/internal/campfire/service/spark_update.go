package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"campfire/internal/campfire/sparkbundle"
	"campfire/internal/shared/buildinfo"
	"campfire/internal/shared/protocol"
	"campfire/internal/shared/sparkrelease"
)

const sparkReleaseCheckTimeout = 20 * time.Second

// SparkUpdater delivers an explicit client upgrade over an authenticated connection.
type SparkUpdater interface {
	UpdateSpark(context.Context, string, string) error
}

// SparkReleaseStatus describes the Spark build bundled with the running panel.
type SparkReleaseStatus struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
	Error         string `json:"error,omitempty"`
}

// ErrSparkUpdate describes a stale, unsupported or unnecessary upgrade request.
var ErrSparkUpdate = errors.New("spark must be connected, support remote updates, and need the selected panel build")

// SparkUpdateService offers only the build bundled with the running panel.
type SparkUpdateService struct {
	sparks  *SparkService
	updater SparkUpdater
	fetch   func(context.Context) (string, error)
	mu      sync.RWMutex
	status  SparkReleaseStatus
	active  map[string]bool
}

// NewSparkUpdateService selects the panel build without contacting external services.
func NewSparkUpdateService(sparks *SparkService, updater SparkUpdater) *SparkUpdateService {
	return &SparkUpdateService{sparks: sparks, updater: updater, fetch: func(context.Context) (string, error) {
		if !sparkbundle.Available() {
			return "", errors.New("spark packages are not bundled")
		}

		return buildinfo.SparkVersion(), nil
	}, active: make(map[string]bool)}
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
		s.status.Error = "The panel’s bundled Spark packages are unavailable."

		return
	}

	if !sparkrelease.ValidVersion(version) {
		s.status.Error = "The release source returned an invalid Spark version."
		return
	}

	s.status.LatestVersion, s.status.Error = version, ""
}

// Start loads the build metadata once; it cannot change during this process.
func (s *SparkUpdateService) Start(ctx context.Context) {
	s.check(ctx)
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

	if s.updater == nil || !spark.Connected || version != s.Status().LatestVersion || !sparkrelease.UpdateAvailable(spark.AgentVersion, version) {
		return ErrSparkUpdate
	}

	if err := protocol.Require(spark.Capabilities, protocol.CapabilitySelfUpdate); err != nil {
		return err
	}

	return s.updater.UpdateSpark(ctx, id, version)
}
