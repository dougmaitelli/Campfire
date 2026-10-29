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
		features = append(features, protocol.CapabilitySelfReinstall)
	}

	return protocol.ProtocolVersion, p.version, features
}
func (p *updateTestPeer) UpdateSpark(_ context.Context, _, version string) error {
	p.calls++
	p.version = version

	return nil
}

func (p *updateTestPeer) ReinstallSpark(ctx context.Context, id, version string) error {
	return p.UpdateSpark(ctx, id, version)
}

func TestSparkReinstallAlwaysUsesBundledBuildAndRequiresSupport(t *testing.T) {
	for _, version := range []string{"v0.3.0", "v0.4.0", "dev-abcdef123"} {
		t.Run(version, func(t *testing.T) {
			peer := &updateTestPeer{version: version, connected: true, capable: true}
			sparks := NewSparkService(&updateTestStore{spark: model.Spark{ID: "node"}}, peer)
			updates := NewSparkUpdateService(sparks, peer).WithFetcher(func(context.Context) (string, error) { return "v0.3.0", nil })
			sparks.Updates = updates
			updates.Start(t.Context())

			if err := updates.Reinstall(t.Context(), "node", "v0.4.0"); !errors.Is(err, ErrSparkReinstall) {
				t.Fatal("non-bundled build accepted", err)
			}

			peer.connected = false

			if err := updates.Reinstall(t.Context(), "node", "v0.3.0"); !errors.Is(err, ErrSparkReinstall) {
				t.Fatal("offline reinstall accepted", err)
			}

			peer.connected, peer.capable = true, false

			if err := updates.Reinstall(t.Context(), "node", "v0.3.0"); err == nil {
				t.Fatal("unsupported reinstall accepted")
			}

			peer.capable = true

			if err := updates.Reinstall(t.Context(), "node", "v0.3.0"); err != nil {
				t.Fatal(err)
			}

			if peer.calls != 1 || peer.version != "v0.3.0" {
				t.Fatal("forced replacement not delivered", peer)
			}
		})
	}
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
		t.Fatal("page reads reloaded the panel build")
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

	updates.WithFetcher(func(context.Context) (string, error) { return "", errors.New("packages unavailable") }).check(t.Context())

	if status := updates.Status(); status.LatestVersion != "v1.1.0" || status.Error == "" {
		t.Fatal("failed check discarded last good release", status)
	}
}

func TestSparkUpdatesFromDevelopmentToRelease(t *testing.T) {
	for _, version := range []string{"dev", "dev-abcdef123"} {
		t.Run(version, func(t *testing.T) {
			peer := &updateTestPeer{version: version, connected: true, capable: true}
			sparks := NewSparkService(&updateTestStore{spark: model.Spark{ID: "node"}}, peer)
			updates := NewSparkUpdateService(sparks, peer).WithFetcher(func(context.Context) (string, error) { return "v0.3.0", nil })
			sparks.Updates = updates
			updates.Start(t.Context())

			spark, err := sparks.Get(t.Context(), "node")
			if err != nil || !spark.AgentUpdateAvailable || spark.LatestAgentVersion != "v0.3.0" {
				t.Fatal("development Spark did not offer the panel release", spark, err)
			}

			if err := updates.Update(t.Context(), "node", "v0.3.0"); err != nil {
				t.Fatal(err)
			}

			spark, err = sparks.Get(t.Context(), "node")
			if err != nil || spark.AgentUpdateAvailable || peer.calls != 1 || peer.version != "v0.3.0" {
				t.Fatal("release update did not clear availability", spark, peer, err)
			}
		})
	}
}
