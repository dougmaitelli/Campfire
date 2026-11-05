package repository_test

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

func TestJobCheckpointRejectsLateAcknowledgementsAndRetainsCancellation(t *testing.T) {
	db, _, _ := authStore(t)
	if err := db.ORM().AutoMigrate(&database.ServerJob{}); err != nil {
		t.Fatal(err)
	}

	job := database.ServerJob{ID: "copy", Type: model.JobCopy, SourceServerID: "source", TargetServerID: "target", SourceSparkID: "one", TargetSparkID: "two", Data: "{}", Status: model.JobRunning, Step: model.JobCopying, Attempt: 1, TotalBytes: 100}
	if err := db.ORM().Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	store := repository.NewJobRepository(db.ORM())
	if err := store.Advance(t.Context(), job, map[string]any{"transferred_bytes": 50}); err != nil {
		t.Fatal(err)
	}

	if err := store.Advance(t.Context(), job, map[string]any{"transferred_bytes": 25}); !errors.Is(err, repository.ErrJobState) {
		t.Fatal("stale offset accepted", err)
	}

	latest, err := store.Get(t.Context(), job.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Change(t.Context(), job.ID, true); err != nil {
		t.Fatal(err)
	}

	if err := store.Advance(t.Context(), latest, map[string]any{"step": model.JobImporting}); !errors.Is(err, repository.ErrJobState) {
		t.Fatal("late acknowledgement overwrote cancellation", err)
	}

	if err := store.Guard(t.Context(), "source"); !errors.Is(err, repository.ErrJobActive) {
		t.Fatal("cancellation released reservation", err)
	}

	latest, err = store.Get(t.Context(), job.ID)
	if err != nil {
		t.Fatal(err)
	}

	if latest.Step != model.JobCopying || latest.Status != model.JobCancelling {
		t.Fatal("cancellation lost checkpoint", latest)
	}

	if err := store.Finish(t.Context(), latest, true); err != nil {
		t.Fatal(err)
	}

	if err := store.Guard(t.Context(), "source"); err != nil {
		t.Fatal(err)
	}
}

func TestJobHistoryPaginationAndEnums(t *testing.T) {
	db, _, _ := authStore(t)
	if err := db.ORM().AutoMigrate(&database.ServerJob{}); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"a", "b", "c"} {
		job := database.ServerJob{ID: id, Type: model.JobTransfer, SourceServerID: "server", TargetServerID: "server", SourceSparkID: "one", TargetSparkID: "two", Status: model.JobCompleted, Step: model.JobCleanup, Attempt: 1, CreatedAt: 10}
		if err := db.ORM().Create(&job).Error; err != nil {
			t.Fatal(err)
		}
	}

	store := repository.NewJobRepository(db.ORM())

	page, err := store.List(t.Context(), "server", []model.JobType{model.JobTransfer}, &repository.JobCursor{CreatedAt: 10, ID: "c"})
	if err != nil || len(page) != 2 || page[0].ID != "b" || page[1].ID != "a" {
		t.Fatal("unstable history ordering", page, err)
	}

	if err := db.ORM().Model(&database.ServerJob{}).Where("id = ?", "a").Update("status", "unknown").Error; err == nil {
		t.Fatal("database accepted invalid status")
	}
}

func TestPlatformJobHistoryCombinesFiltersAndKeepsDeletedEndpoints(t *testing.T) {
	db, _, _ := authStore(t)

	jobs := []database.ServerJob{
		{ID: "pending-copy", Type: model.JobCopy, Status: model.JobPending, Step: model.JobPreparing},
		{ID: "running-copy", Type: model.JobCopy, Status: model.JobRunning, Step: model.JobCopying},
		{ID: "done-copy", Type: model.JobCopy, Status: model.JobCompleted, Step: model.JobCleanup},
		{ID: "done-transfer", Type: model.JobTransfer, Status: model.JobCompleted, Step: model.JobCleanup},
	}
	for index, job := range jobs {
		job.SourceServerID, job.TargetServerID = fmt.Sprintf("source-%d", index), fmt.Sprintf("target-%d", index)

		job.SourceSparkID, job.TargetSparkID, job.Attempt, job.CreatedAt = "source", "target", 1, int64(index)
		if err := db.ORM().Create(&job).Error; err != nil {
			t.Fatal(err)
		}
	}

	store := repository.NewJobRepository(db.ORM())

	all, err := store.ListAll(t.Context(), repository.JobFilter{}, nil)
	if err != nil || len(all) != 4 || all[0].SourceName != "source-3" {
		t.Fatal("platform history lost a server or deleted endpoint", all, err)
	}

	filtered, err := store.ListAll(t.Context(), repository.JobFilter{Type: model.JobCopy, Status: model.JobCompleted}, nil)
	if err != nil || len(filtered) != 1 || filtered[0].ID != "done-copy" {
		t.Fatal("platform filters did not combine", filtered, err)
	}

	older, err := store.ListAll(t.Context(), repository.JobFilter{Type: model.JobCopy}, &repository.JobCursor{CreatedAt: 1, ID: "running-copy"})
	if err != nil || len(older) != 1 || older[0].ID != "pending-copy" {
		t.Fatal("platform filter cursor lost older jobs", older, err)
	}
}

func TestJobHistoryBatchesNamesAndRetainsDeletedEndpoints(t *testing.T) {
	db, _, _ := authStore(t)
	for _, row := range []any{
		&database.Spark{ID: "spark", Name: "Current Spark"},
		&database.Ember{ID: "ember", Name: "Game", Definition: model.Ember{Schema: 1}},
		&database.Server{ID: "server", Name: "Current server", SparkID: "spark", EmberID: "ember"},
	} {
		if err := db.ORM().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	for index := 0; index < 50; index++ {
		job := database.ServerJob{ID: fmt.Sprint("job-", index), Type: model.JobCopy, SourceServerID: "server", TargetServerID: "deleted", SourceSparkID: "spark", TargetSparkID: "deleted-spark", Status: model.JobCompleted, Step: model.JobCleanup, Attempt: 1}
		if err := db.ORM().Create(&job).Error; err != nil {
			t.Fatal(err)
		}
	}

	var reads atomic.Int64

	if err := db.ORM().Callback().Query().After("gorm:query").Register("job_history_count", func(*gorm.DB) { reads.Add(1) }); err != nil {
		t.Fatal(err)
	}

	store := repository.NewJobRepository(db.ORM())

	jobs, err := store.List(t.Context(), "server", []model.JobType{model.JobCopy}, nil)
	if err != nil || len(jobs) != 50 {
		t.Fatal("history is incomplete", len(jobs), err)
	}

	if reads.Load() != 1 {
		t.Fatal("history performed per-job metadata reads", reads.Load())
	}

	for _, job := range jobs {
		if job.SourceName != "Current server" || job.SourceSparkName != "Current Spark" || job.TargetName != "deleted" || job.TargetSparkName != "deleted-spark" {
			t.Fatal("history lost endpoint metadata", job)
		}
	}

	item, err := store.HistoryItem(t.Context(), jobs[0].ID)
	if err != nil || item.SourceName != jobs[0].SourceName || item.TargetName != jobs[0].TargetName {
		t.Fatal("individual history projection differs", item, err)
	}
}

func TestRunnableJobsDoNotStarveHealthyWorkBehindOfflineCleanup(t *testing.T) {
	db, _, _ := authStore(t)
	store := repository.NewJobRepository(db.ORM())

	for _, job := range []database.ServerJob{
		{ID: "offline-cleanup", Type: model.JobCopy, SourceServerID: "one", TargetServerID: "two", SourceSparkID: "first", TargetSparkID: "second", Status: model.JobCancelling, Step: model.JobCleanup, Attempt: 1, Error: "destination offline", CreatedAt: 1, UpdatedAt: 3},
		{ID: "healthy", Type: model.JobCopy, SourceServerID: "three", TargetServerID: "four", SourceSparkID: "third", TargetSparkID: "fourth", Status: model.JobPending, Step: model.JobPreparing, Attempt: 1, CreatedAt: 2, UpdatedAt: 2},
	} {
		if err := db.ORM().Create(&job).Error; err != nil {
			t.Fatal(err)
		}
	}

	jobs, err := store.Runnable(t.Context())
	if err != nil || len(jobs) != 2 || jobs[0].ID != "healthy" || jobs[1].ID != "offline-cleanup" {
		t.Fatal("offline cleanup starved healthy dispatch", jobs, err)
	}
}
