package service

import (
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

func TestSavedataSchedulesValidateDeclarationsAndKeepModeOnRetry(t *testing.T) {
	s, db, _, principal := activityFixture(t)
	ctx := t.Context()
	input := database.Schedule{ServerID: "game", Name: "Save data", Action: "backup", BackupMode: model.BackupModeSavedata, CronExpression: "0 3 * * *", TimeZone: "UTC"}

	if _, err := s.Save(ctx, principal, input, true); err == nil {
		t.Fatal("accepted save-data schedule without Ember declarations")
	}

	embers := repository.NewEmberRepository(db.ORM())

	ember, err := embers.Get(ctx, "ember")
	if err != nil {
		t.Fatal(err)
	}

	ember.SavedataPaths = []model.SavedataPath{{Path: "world", Required: true}}
	if err := embers.Replace(ctx, ember); err != nil {
		t.Fatal(err)
	}

	item, err := s.Save(ctx, principal, input, true)
	if err != nil {
		t.Fatal(err)
	}

	item.Name = "Updated save-data schedule"
	if _, err := s.Save(ctx, principal, item, false); err != nil {
		t.Fatal(err)
	}

	run, err := s.Store.ClaimSchedule(ctx, item.ID, time.Now(), &principal.UserID)
	if err != nil || run.BackupMode != model.BackupModeSavedata {
		t.Fatal("schedule did not capture its backup mode", run, err)
	}

	if err := s.Store.UpdateRun(ctx, run.ID, "failed", "test failure", time.Now()); err != nil {
		t.Fatal(err)
	}

	retried, err := s.Retry(ctx, principal, run.ID)
	if err != nil || retried.BackupMode != model.BackupModeSavedata {
		t.Fatal("retry changed backup mode", retried, err)
	}

	ember.SavedataPaths = nil
	if err := embers.Replace(ctx, ember); err != nil {
		t.Fatal(err)
	}

	if _, err := s.deployment.backupOptions(ctx, "game", model.BackupModeSavedata); err == nil {
		t.Fatal("removed declarations silently allowed a scheduled save-data backup")
	}

	options, err := s.deployment.backupOptions(ctx, "game", model.BackupModeFull)
	if err != nil || options.Mode != model.BackupModeFull || len(options.Paths) != 0 {
		t.Fatal("full mode must remain available without save-data declarations", options, err)
	}
}

func TestBackupRetentionCountsEachModeSeparately(t *testing.T) {
	now := time.Now()
	rows := []database.Backup{
		{Name: "save-new", Mode: model.BackupModeSavedata, Local: true, CreatedAt: now.UnixNano()},
		{Name: "save-old", Mode: model.BackupModeSavedata, Local: true, CreatedAt: now.Add(-time.Hour).UnixNano()},
		{Name: "full-new", Mode: model.BackupModeFull, Local: true, CreatedAt: now.Add(-2 * time.Hour).UnixNano()},
		{Name: "full-legacy", Local: true, CreatedAt: now.Add(-3 * time.Hour).UnixNano()},
	}
	selected := remoteCandidates(rows, model.BackupPolicy{Storage: "s3", RemoteKeepLast: 1, Savedata: model.BackupRetention{RemoteKeepLast: 1}}, now)

	if len(selected) != 2 || !selected["save-new"] || !selected["full-new"] {
		t.Fatal("save-data snapshots displaced full snapshots", selected)
	}

	selected = remoteCandidates(rows, model.BackupPolicy{Storage: "s3", RemoteKeepLast: 1, Savedata: model.BackupRetention{RemoteKeepLast: 2}}, now)
	if len(selected) != 3 || !selected["save-old"] || selected["full-legacy"] {
		t.Fatal("mode-specific retention limits were ignored", selected)
	}
}

func TestBackupRetentionPolicyPersistsIndependentLimits(t *testing.T) {
	f, service, _, _ := backupFixture(t)
	policy := model.BackupPolicy{Storage: "local", LocalKeepLast: 3, LocalMaxAgeDays: 30, RemoteKeepLast: 5, RemoteMaxAgeDays: 90, Savedata: model.BackupRetention{LocalKeepLast: 12, LocalMaxAgeDays: 7, RemoteKeepLast: 24, RemoteMaxAgeDays: 14}}

	if err := service.SavePolicy(t.Context(), f.server.ID, policy); err != nil {
		t.Fatal(err)
	}

	saved, err := service.Store.Policy(t.Context(), f.server.ID)
	if err != nil || saved != policy {
		t.Fatal("mode-specific retention did not survive storage", saved, err)
	}

	policy.Savedata.LocalKeepLast = -1
	if err := service.SavePolicy(t.Context(), f.server.ID, policy); err == nil {
		t.Fatal("negative save-data retention was accepted")
	}
}

func TestLocalBackupRetentionUsesEachModesLimit(t *testing.T) {
	f, service, _, _ := backupFixture(t)
	ctx := t.Context()

	for index, item := range []struct {
		name string
		mode model.BackupMode
	}{
		{"full-new", model.BackupModeFull},
		{"save-new", model.BackupModeSavedata},
		{"save-middle", model.BackupModeSavedata},
		{"full-old", model.BackupModeFull},
		{"save-old", model.BackupModeSavedata},
	} {
		created := time.Now().Add(-time.Duration(index) * time.Hour)
		seedBackup(t, f, item.name, created)

		if err := service.Store.Discover(ctx, database.Backup{ServerID: f.server.ID, Name: item.name + ".tar.gz", Mode: item.mode, Local: true, CreatedAt: created.UnixNano(), Size: int64(len(f.world)), Key: item.name}); err != nil {
			t.Fatal(err)
		}
	}

	if err := service.SavePolicy(ctx, f.server.ID, model.BackupPolicy{Storage: "local", LocalKeepLast: 1, Savedata: model.BackupRetention{LocalKeepLast: 2}}); err != nil {
		t.Fatal(err)
	}

	if err := service.reconcile(ctx, f.server.ID); err != nil {
		t.Fatal(err)
	}

	rows, err := service.Store.List(ctx, f.server.ID)
	if err != nil || len(rows) != 3 {
		t.Fatal("unexpected retained backup count", rows, err)
	}

	for _, row := range rows {
		if row.Name == "full-old.tar.gz" || row.Name == "save-old.tar.gz" {
			t.Fatal("expired backup survived retention", row.Name)
		}
	}
}
