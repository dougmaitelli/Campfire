package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestBackupMetadataCanChangeDuringDiscovery(t *testing.T) {
	f, s, _, _ := backupFixture(t)
	seedBackup(t, f, "snapshot", time.Now())

	if err := s.discover(t.Context(), f.server.ID); err != nil {
		t.Fatal(err)
	}

	release, err := s.acquire(f.server.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	updated, err := s.Update(t.Context(), f.server.ID, "snapshot.tar.gz", model.BackupChanges{DisplayName: new("Before update"), Pinned: new(true)})
	if err != nil || !updated.Pinned || updated.DisplayName != "Before update" || updated.State != model.BackupAvailable {
		t.Fatal("discovery blocked metadata or changed availability", updated, err)
	}
}

func TestPinnedBackupsSurviveRetentionWithoutUsingNormalSlots(t *testing.T) {
	f, s, remote, router := backupFixture(t)
	ctx := t.Context()
	now := time.Now()

	for _, mode := range []model.BackupMode{model.BackupModeFull, model.BackupModeSavedata} {
		for _, item := range []struct {
			name string
			age  time.Duration
		}{
			{"old-pinned", 30 * 24 * time.Hour},
			{"newest-pinned", 0},
			{"recent", time.Hour},
			{"expired", 10 * 24 * time.Hour},
		} {
			seedBackup(t, f, string(mode)+"-"+item.name, now.Add(-item.age))
		}
	}

	if err := s.discover(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	for _, mode := range []model.BackupMode{model.BackupModeFull, model.BackupModeSavedata} {
		for _, name := range []string{"old-pinned", "newest-pinned", "recent", "expired"} {
			archive := fmt.Sprintf("%s-%s.tar.gz", mode, name)
			if err := s.Store.Update(ctx, f.server.ID, archive, map[string]any{"mode": mode}); err != nil {
				t.Fatal(err)
			}

			if name == "old-pinned" || name == "newest-pinned" {
				router.offline = true
				_, err := s.Update(ctx, f.server.ID, archive, model.BackupChanges{DisplayName: new("Before update"), Pinned: new(true)})
				router.offline = false

				if err != nil {
					t.Fatal("offline metadata update failed", err)
				}
			}
		}
	}

	retention := model.BackupRetention{LocalKeepLast: 1, RemoteKeepLast: 1, LocalMaxAgeDays: 1, RemoteMaxAgeDays: 1}
	policy := model.BackupPolicy{Storage: model.BackupStorageS3, LocalKeepLast: 1, RemoteKeepLast: 1, LocalMaxAgeDays: 1, RemoteMaxAgeDays: 1, Savedata: retention}

	if err := s.SavePolicy(ctx, f.server.ID, policy); err != nil {
		t.Fatal(err)
	}

	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	rows, err := s.Store.List(ctx, f.server.ID)
	if err != nil || len(rows) != 6 || len(remote.objects) != 6 {
		t.Fatal("pins consumed ordinary retention slots or failed to sync", rows, len(remote.objects), err)
	}

	for _, row := range rows {
		if !row.Local || !row.Remote || row.Pinned && row.DisplayName != "Before update" {
			t.Fatal("retention removed a copy or discovery reset metadata", row)
		}
	}

	name := "full-old-pinned.tar.gz"
	for _, location := range []model.BackupStorage{model.BackupStorageLocal, model.BackupStorageS3, model.BackupStorageAll} {
		if err := s.Delete(ctx, f.server.ID, name, location); err == nil {
			t.Fatal("pinned backup accepted deletion", location)
		}
	}

	if _, err := s.Update(ctx, f.server.ID, name, model.BackupChanges{Pinned: new(false)}); err != nil {
		t.Fatal(err)
	}

	if err := s.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Store.Get(ctx, f.server.ID, name); !errors.Is(err, repository.ErrAccessNotFound) {
		t.Fatal("unpinning did not restore normal retention", err)
	}
}
