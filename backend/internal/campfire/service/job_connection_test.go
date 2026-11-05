package service

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func queuedJob(t *testing.T, mode string) (*transferFixture, *CopyService, database.ServerJob) {
	t.Helper()

	f, copies, _ := newCopyFixture(t)
	f.router.targetOffline = true
	store := copies.Store.Jobs()

	if mode == "transfer" {
		if _, err := f.service.Begin(t.Context(), f.server.ID, "target", f.ports); err != nil {
			t.Fatal(err)
		}
	} else {
		input := f.server.ServerConfiguration

		input.Name, input.SparkID = "Queued copy", "target"
		if _, err := copies.Create(t.Context(), f.server.ID, input, 1, mode == "quick", true, nil); err != nil {
			t.Fatal(err)
		}
	}

	jobs, err := store.Runnable(t.Context())
	if err != nil || len(jobs) != 1 {
		t.Fatal("queued job is missing", jobs, err)
	}

	return f, copies, jobs[0]
}

func waitJobStatus(t *testing.T, store *repository.JobRepository, id string, status model.JobStatus) database.ServerJob {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		job, err := store.Get(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}

		if job.Status == status {
			return job
		}

		if job.Status == model.JobFailed {
			t.Fatal("job failed", job.Error)
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("job did not reach", status)

	return database.ServerJob{}
}

func TestJobsWaitForConnectionAndResumeAfterPanelRestart(t *testing.T) {
	for _, mode := range []string{"copy", "quick", "transfer"} {
		t.Run(mode, func(t *testing.T) {
			f, copies, job := queuedJob(t, mode)
			d := copies.Deployment
			handler := workflowHandler{d, job.Type}
			runner := NewJobService(copies.Store.Jobs(), map[model.JobType]JobHandler{job.Type: handler})
			runner.ctx = t.Context()
			runner.pump()

			if len(runner.running) != 0 {
				t.Fatal("offline job consumed a worker")
			}

			stored, err := runner.Store.Get(t.Context(), job.ID)
			if err != nil || stored.Status != model.JobPending || stored.Step != model.JobPreparing || stored.Error != "" {
				t.Fatal("offline job lost its pending checkpoint", stored, err)
			}

			if !slices.Equal(d.JobWaitingFor(stored), []string{"target"}) {
				t.Fatal("missing destination wait reason")
			}

			if err := runner.Store.Guard(t.Context(), f.server.ID); !errors.Is(err, repository.ErrJobActive) {
				t.Fatal("waiting job lost its reservation", err)
			}

			// A fresh coordinator discovers the persisted job after reconnection.
			f.router.mu.Lock()
			f.router.targetOffline = false
			f.router.mu.Unlock()
			d.StartJobs(t.Context())
			t.Cleanup(d.CloseJobs)

			finished := waitJobStatus(t, runner.Store, job.ID, model.JobCompleted)
			if finished.Attempt != job.Attempt || finished.Error != "" {
				t.Fatal("connection wait required a retry", finished)
			}
		})
	}
}

func TestQueuedJobsCancelWithoutConnecting(t *testing.T) {
	for _, mode := range []string{"copy", "quick", "transfer"} {
		t.Run(mode, func(t *testing.T) {
			f, copies, job := queuedJob(t, mode)

			store := copies.Store.Jobs()
			if err := store.Change(t.Context(), job.ID, true); err != nil {
				t.Fatal(err)
			}

			cancelled, err := store.Get(t.Context(), job.ID)
			if err != nil || cancelled.Status != model.JobCancelled || cancelled.FinishedAt == nil {
				t.Fatal("queued cancellation waited for a Spark", cancelled, err)
			}

			if err := store.Guard(t.Context(), f.server.ID); err != nil {
				t.Fatal("queued cancellation retained its reservation", err)
			}

			if mode != "transfer" {
				if _, err := copies.Deployment.Servers.Get(t.Context(), job.TargetServerID); !errors.Is(err, ErrServerNotFound) {
					t.Fatal("cancelled copy retained its destination", err)
				}
			}

			if _, err := copies.Deployment.Servers.Get(t.Context(), f.server.ID); err != nil {
				t.Fatal("queued cancellation removed the source", err)
			}
		})
	}
}

func TestQueuedJobsCheckCapabilitiesBeforeDeliveringOnConnection(t *testing.T) {
	for _, mode := range []string{"copy", "quick", "transfer"} {
		t.Run(mode, func(t *testing.T) {
			f, copies, job := queuedJob(t, mode)
			router := &jobCapabilityRouter{transferRouter: f.router, missing: protocol.CapabilityLifecycle}
			copies.Deployment.Remote = router
			f.router.targetOffline = false

			copies.Deployment.StartJobs(t.Context())
			t.Cleanup(copies.Deployment.CloseJobs)

			failed := waitJobStatus(t, copies.Store.Jobs(), job.ID, model.JobFailed)
			if failed.Step != model.JobPreparing || failed.Error == "" {
				t.Fatal("unsupported destination lost its checkpoint", failed)
			}

			copies.Deployment.CloseJobs()

			if router.deliveries != 0 {
				t.Fatal("preparation delivered configuration before checking both endpoints")
			}
		})
	}
}

type disconnectingJobRouter struct {
	*transferRouter
	once         sync.Once
	disconnected chan struct{}
}

func (r *disconnectingJobRouter) Job(ctx context.Context, spark string, request model.JobRequest) (model.JobResult, error) {
	if request.Operation == model.JobOperationWrite {
		dropped := false

		r.once.Do(func() {
			r.mu.Lock()
			r.targetOffline = true
			r.mu.Unlock()

			dropped = true

			close(r.disconnected)
		})

		if dropped {
			return model.JobResult{}, errors.New("connection lost during archive upload")
		}
	}

	return r.transferRouter.Job(ctx, spark, request)
}

func TestJobsResumeDisconnectedUploadWithoutRetry(t *testing.T) {
	for _, mode := range []string{"quick", "transfer"} {
		t.Run(mode, func(t *testing.T) {
			f, copies, job := queuedJob(t, mode)
			f.router.targetOffline = false
			router := &disconnectingJobRouter{transferRouter: f.router, disconnected: make(chan struct{})}
			d := copies.Deployment
			d.Remote = router
			d.StartJobs(t.Context())
			t.Cleanup(d.CloseJobs)

			select {
			case <-router.disconnected:
			case <-time.After(10 * time.Second):
				t.Fatal("job did not begin uploading")
			}

			d.CloseJobs()

			paused, err := copies.Store.Jobs().Get(t.Context(), job.ID)
			if err != nil || paused.Status != model.JobRunning || paused.Step != model.JobCopying || paused.Error != "" {
				t.Fatal("disconnect failed the job or lost its checkpoint", paused, err)
			}

			f.router.mu.Lock()
			f.router.targetOffline = false
			f.router.mu.Unlock()
			d.StartJobs(t.Context())

			completed := waitJobStatus(t, copies.Store.Jobs(), job.ID, model.JobCompleted)
			if completed.Attempt != job.Attempt {
				t.Fatal("reconnection required a new attempt", completed)
			}
		})
	}
}
