package service

import (
	"context"
	"errors"
	"testing"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

type updateTestStore struct{ spark model.Spark }

func (s *updateTestStore) Create(context.Context, model.Spark) error { return nil }
func (s *updateTestStore) List(context.Context) ([]model.Spark, error) {
	return []model.Spark{s.spark}, nil
}
func (s *updateTestStore) Get(context.Context, string) (model.Spark, error) { return s.spark, nil }
func (s *updateTestStore) Update(context.Context, string, model.SparkChanges) (model.Spark, error) {
	return s.spark, nil
}
func (s *updateTestStore) Delete(context.Context, string) error { return nil }

type updateTestPeer struct {
	version            string
	connected, capable bool
	calls              int
}

func (p *updateTestPeer) Connected(string) bool { return p.connected }
func (p *updateTestPeer) SparkFeatures(string) (int, string, []string) {
	features := []string{}
	if p.capable {
		features = append(features, protocol.CapabilitySelfUpdate)
	}

	return 7, p.version, features
}
func (p *updateTestPeer) UpdateSpark(_ context.Context, _, version string) error {
	p.calls++
	p.version = version

	return nil
}

func TestSparkUpdatesUseCachedVersionsAndRejectUnsupportedRequests(t *testing.T) {
	peer := &updateTestPeer{version: "v1.0.0", connected: true, capable: true}
	sparks := NewSparkService(&updateTestStore{spark: model.Spark{ID: "node"}}, peer)
	fetches := 0
	updates := NewSparkUpdateService(sparks, peer).WithFetcher(func(context.Context) (string, error) { fetches++; return "v1.1.0", nil })
	sparks.Updates = updates
	updates.check(t.Context())

	for range 5 {
		spark, err := sparks.Get(t.Context(), "node")
		if err != nil || !spark.AgentUpdateAvailable {
			t.Fatal(spark, err)
		}
	}

	if fetches != 1 {
		t.Fatal("page reads fetched GitHub again")
	}

	if err := updates.Update(t.Context(), "node", "v1.2.0"); !errors.Is(err, ErrSparkUpdate) {
		t.Fatal("stale release accepted", err)
	}

	peer.capable = false

	if err := updates.Update(t.Context(), "node", "v1.1.0"); err == nil {
		t.Fatal("unsupported client updated")
	}

	peer.capable, peer.connected = true, false

	if err := updates.Update(t.Context(), "node", "v1.1.0"); !errors.Is(err, ErrSparkUpdate) {
		t.Fatal("offline client updated", err)
	}

	peer.connected = true

	if err := updates.Update(t.Context(), "node", "v1.1.0"); err != nil {
		t.Fatal(err)
	}

	if peer.calls != 1 || peer.version != "v1.1.0" {
		t.Fatal("update not delivered", peer)
	}

	if err := updates.Update(t.Context(), "node", "v1.1.0"); !errors.Is(err, ErrSparkUpdate) {
		t.Fatal("equal version updated", err)
	}

	updates.WithFetcher(func(context.Context) (string, error) { return "", errors.New("GitHub unavailable") }).check(t.Context())

	if status := updates.Status(); status.LatestVersion != "v1.1.0" || status.Error == "" {
		t.Fatal("failed check discarded last good release", status)
	}
}
