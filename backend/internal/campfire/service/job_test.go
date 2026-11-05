package service

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

type blockedJobHandler struct {
	mu      sync.Mutex
	started []database.ServerJob
}

func (h *blockedJobHandler) Ready(job database.ServerJob) bool {
	return job.TargetSparkID != "offline"
}

func claimJob(t *testing.T, store *repository.JobRepository, id string) {
	t.Helper()

	job, err := store.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	if job.Status == model.JobPending {
		if err := store.Advance(t.Context(), job, map[string]any{"status": model.JobRunning}); err != nil {
			t.Fatal(err)
		}
	}
}

func (h *blockedJobHandler) Step(ctx context.Context, job database.ServerJob) (bool, error) {
	h.mu.Lock()
	h.started = append(h.started, job)
	h.mu.Unlock()
	<-ctx.Done()

	return false, ctx.Err()
}

func (h *blockedJobHandler) Cancel(ctx context.Context, job database.ServerJob) (bool, error) {
	return h.Step(ctx, job)
}

func TestJobRunnerBoundsWorkAndPreservesCheckpointsOnShutdown(t *testing.T) {
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	if err := db.ORM().AutoMigrate(&database.ServerJob{}); err != nil {
		t.Fatal(err)
	}

	jobs := []database.ServerJob{
		{ID: "0-offline-a", SourceSparkID: "one", TargetSparkID: "offline"},
		{ID: "0-offline-b", SourceSparkID: "two", TargetSparkID: "offline"},
		{ID: "0-offline-c", SourceSparkID: "three", TargetSparkID: "offline"},
		{ID: "0-offline-d", SourceSparkID: "four", TargetSparkID: "offline"},
		{ID: "a", SourceSparkID: "one", TargetSparkID: "one"},
		{ID: "b", SourceSparkID: "one", TargetSparkID: "two"},
		{ID: "c", SourceSparkID: "one", TargetSparkID: "three"},
		{ID: "d", SourceSparkID: "three", TargetSparkID: "four"},
		{ID: "e", SourceSparkID: "four", TargetSparkID: "five"},
		{ID: "f", SourceSparkID: "six", TargetSparkID: "seven"},
	}
	for _, job := range jobs {
		job.Type, job.Status, job.Step, job.Attempt = model.JobCopy, model.JobPending, model.JobPreparing, 1

		job.SourceServerID, job.TargetServerID, job.Data = job.ID+"-source", job.ID+"-target", "{}"
		if err := db.ORM().Create(&job).Error; err != nil {
			t.Fatal(err)
		}
	}

	handler := &blockedJobHandler{}
	store := repository.NewJobRepository(db.ORM())
	runner := NewJobService(store, map[model.JobType]JobHandler{model.JobCopy: handler})
	runner.Start(t.Context())
	t.Cleanup(runner.Close)

	deadline := time.Now().Add(5 * time.Second)

	for {
		handler.mu.Lock()
		started := append([]database.ServerJob(nil), handler.started...)
		handler.mu.Unlock()

		if len(started) == 4 {
			counts := map[string]int{}
			for _, job := range started {
				counts[job.SourceSparkID]++
				if job.TargetSparkID != job.SourceSparkID {
					counts[job.TargetSparkID]++
				}
			}

			for spark, count := range counts {
				if count > 2 {
					t.Fatal("Spark concurrency exceeded", spark, count)
				}
			}

			break
		}

		if time.Now().After(deadline) {
			t.Fatal("runner did not dispatch four independent jobs", started)
		}

		time.Sleep(10 * time.Millisecond)
	}

	runner.Close()

	for _, original := range jobs {
		job, err := store.Get(t.Context(), original.ID)
		if err != nil {
			t.Fatal(err)
		}

		if job.Status != model.JobPending && job.Status != model.JobRunning {
			t.Fatal("shutdown changed recoverable state", job)
		}

		if job.Step != model.JobPreparing || job.Attempt != 1 {
			t.Fatal("shutdown lost checkpoint", job)
		}

		if original.TargetSparkID == "offline" && job.Status != model.JobPending {
			t.Fatal("offline job was dispatched", job)
		}
	}
}

func TestCopyJobProtectsEmberUntilCancellationCleanup(t *testing.T) {
	f, copies, _ := newCopyFixture(t)
	input := f.server.ServerConfiguration
	input.Name = "Protected copy"

	target, err := copies.Create(t.Context(), f.server.ID, input, 1, true, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	ember, err := copies.Deployment.Embers.Get(t.Context(), f.server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	ember.Startup = "sleep 1800"
	if _, err := copies.Deployment.Embers.Replace(t.Context(), ember.ID, ember); !errors.Is(err, repository.ErrJobActive) {
		t.Fatal("job allowed its definition to change", err)
	}

	job := copyJob(t, copies, target.ID)

	job = runCopyUntil(t, copies, job.ID, model.CopyBackup)
	if err := copies.Store.Change(t.Context(), job.ID, true); err != nil {
		t.Fatal(err)
	}

	if _, err := copies.Deployment.Embers.Replace(t.Context(), ember.ID, ember); !errors.Is(err, repository.ErrJobActive) {
		t.Fatal("cancellation released definition early", err)
	}

	runCopyUntil(t, copies, job.ID, model.CopyCancelled)

	if _, err := copies.Deployment.Embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal("completed cleanup retained definition lock", err)
	}
}

func TestDefinitionLockSupportsNestedCreationAndCancellation(t *testing.T) {
	ctx, release, err := holdDefinition(t.Context(), t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	_, nested, err := holdDefinition(ctx, t.Name())
	if err != nil {
		t.Fatal(err)
	}

	nested()

	waiting, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	if _, _, err := holdDefinition(waiting, t.Name()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("competing definition writer was not protected", err)
	}

	_, other, err := holdDefinition(t.Context(), t.Name()+"-other")
	if err != nil {
		t.Fatal(err)
	}

	other()
	release()

	_, again, err := holdDefinition(t.Context(), t.Name())
	if err != nil {
		t.Fatal(err)
	}

	again()
	definitionLocks.Lock()
	_, retained := definitionLocks.entries[t.Name()]
	definitionLocks.Unlock()

	if retained {
		t.Fatal("definition lock retained an unused entry")
	}
}

func TestJobTimeoutPreservesCheckpointAndCancellation(t *testing.T) {
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "timeout.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	store := repository.NewJobRepository(db.ORM())
	runner := NewJobService(store, nil)
	runner.ctx = t.Context()

	expired, cancel := context.WithCancel(t.Context())
	cancel()

	for _, status := range []model.JobStatus{model.JobRunning, model.JobCancelling} {
		job := database.ServerJob{ID: string(status), Type: model.JobCopy, SourceServerID: "source", TargetServerID: "target", SourceSparkID: "one", TargetSparkID: "two", Data: "{}", Status: status, Step: model.JobCopying, Attempt: 1, TransferredBytes: 50, TotalBytes: 100}
		if err := db.ORM().Create(&job).Error; err != nil {
			t.Fatal(err)
		}

		runner.expire(expired, job.ID)

		updated, err := store.Get(t.Context(), job.ID)
		if err != nil {
			t.Fatal(err)
		}

		want := model.JobFailed
		if status == model.JobCancelling {
			want = model.JobCancelling
		}

		if updated.Status != want || updated.Step != job.Step || updated.TransferredBytes != 50 || updated.Error == "" || !updated.Active() {
			t.Fatal("timeout lost recovery state", updated)
		}
	}
}
