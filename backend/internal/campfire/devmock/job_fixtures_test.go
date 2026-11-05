//go:build !production

package devmock

import (
	"context"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func TestJobExamplesReuseFleetAndCoverLifecycle(t *testing.T) {
	runtime, db, auth, servers := fixture(t)
	if err := runtime.EnableJobExamples(t.Context()); err != nil {
		t.Fatal(err)
	}

	known := make(map[string]model.Server, len(servers))
	for _, server := range servers {
		known[server.ID] = server
	}

	store := repository.NewJobRepository(db.ORM())

	jobs, err := store.ListAll(t.Context(), repository.JobFilter{}, nil)
	if err != nil || len(jobs) != 8 {
		t.Fatal("missing copy and transfer examples", jobs, err)
	}

	states := make(map[model.JobStatus]bool)
	types := make(map[model.JobType]bool)
	active := make(map[string]string)

	for _, job := range jobs {
		states[job.Status], types[job.Type] = true, true
		for _, id := range []string{job.SourceServerID, job.TargetServerID} {
			if _, exists := known[id]; !exists {
				t.Fatal("example invented a server", job.ID, id)
			}

			if job.Active() {
				if owner, exists := active[id]; exists && owner != job.ID {
					t.Fatal("examples reserve the same server twice", owner, job.ID)
				}

				active[id] = job.ID
			}
		}

		if !job.Active() && job.FinishedAt == nil {
			t.Fatal("terminal example has no completion time", job.ID)
		}

		if job.ID == pendingJobExampleID && runtime.Connected(job.SourceSparkID) {
			t.Fatal("queued example does not cover a disconnected Spark")
		}

		if job.Active() && job.Type == model.JobTransfer {
			data, err := job.TransferData()
			if err != nil || len(data.TargetPorts) != len(known[job.SourceServerID].Ports) {
				t.Fatal("transfer example has an invalid placement checkpoint", job.ID, err)
			}
		}
	}

	for _, state := range []model.JobStatus{model.JobPending, model.JobRunning, model.JobFailed, model.JobCompleted, model.JobCancelled} {
		if !states[state] {
			t.Fatal("missing job state", state)
		}
	}

	if !types[model.JobCopy] || !types[model.JobTransfer] {
		t.Fatal("missing a job type", types)
	}

	restarted, err := New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	if err := restarted.EnableJobExamples(t.Context()); err != nil {
		t.Fatal(err)
	}

	reloaded, err := store.ListAll(t.Context(), repository.JobFilter{}, nil)
	if err != nil || len(reloaded) != len(jobs) {
		t.Fatal("restart duplicated the examples", reloaded, err)
	}

	after, err := repository.NewServerRepository(db.ORM()).List(t.Context())
	if err != nil || len(after) != len(servers) {
		t.Fatal("examples changed the fleet", after, err)
	}
}

func TestJobExampleActionsPreserveExistingServers(t *testing.T) {
	runtime, db, _, servers := fixture(t)
	if err := runtime.EnableJobExamples(t.Context()); err != nil {
		t.Fatal(err)
	}

	transfers := mockTransferService(t, runtime, db)
	d := transfers.Deployment
	d.Copies = service.NewCopyService(repository.NewCopyRepository(db.ORM()), d)
	d.StartJobs(t.Context())
	t.Cleanup(d.CloseJobs)

	store := transfers.Store.Jobs()

	if err := store.Change(t.Context(), failedJobExampleID, false); err != nil {
		t.Fatal(err)
	}

	retried, err := store.Get(t.Context(), failedJobExampleID)
	if err != nil || retried.Status != model.JobRunning || retried.Attempt != 2 {
		t.Fatal("example cannot be retried", retried, err)
	}

	for _, id := range []string{pendingJobExampleID, runningJobExampleID, failedJobExampleID} {
		if err := store.Change(t.Context(), id, true); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(5 * time.Second)

	for {
		finished := true

		for _, id := range []string{pendingJobExampleID, runningJobExampleID, failedJobExampleID} {
			job, err := store.Get(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}

			finished = finished && job.Status == model.JobCancelled
		}

		if finished {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("example cancellation did not finish")
		}

		time.Sleep(10 * time.Millisecond)
	}

	for _, source := range servers {
		server, err := d.Servers.Get(t.Context(), source.ID)
		if err != nil || server.SparkID != source.SparkID {
			t.Fatal("example action removed or moved a reused server", source.ID, server, err)
		}

		if err := store.Guard(t.Context(), source.ID); err != nil {
			t.Fatal("cancelled example retained the server lock", source.ID, err)
		}
	}
}

func TestJobExamplesDoNotRecreateDeletedServers(t *testing.T) {
	runtime, db, _, _ := fixture(t)
	if err := db.ORM().Where("id = ?", "creative").Delete(&database.Server{}).Error; err != nil {
		t.Fatal(err)
	}

	if err := runtime.EnableJobExamples(t.Context()); err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.ORM().Model(&database.Server{}).Where("id = ?", "creative").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("examples recreated a deleted server", count, err)
	}
}

type exampleDelegate struct{ calls int }

func (h *exampleDelegate) Ready(database.ServerJob) bool {
	h.calls++

	return true
}

func (h *exampleDelegate) Step(context.Context, database.ServerJob) (bool, error) {
	h.calls++

	return false, nil
}

func (h *exampleDelegate) Cancel(context.Context, database.ServerJob) (bool, error) {
	h.calls++

	return false, nil
}

func TestExampleJobHandlerDelegatesUserCreatedWork(t *testing.T) {
	runtime, _, _, _ := fixture(t)
	if err := runtime.EnableJobExamples(t.Context()); err != nil {
		t.Fatal(err)
	}

	next := &exampleDelegate{}
	handler := runtime.WrapJobHandler(next)

	job := database.ServerJob{ID: "user-created-job"}
	if !handler.Ready(job) {
		t.Fatal("ordinary job was held as an example")
	}

	if _, err := handler.Step(t.Context(), job); err != nil {
		t.Fatal(err)
	}

	if _, err := handler.Cancel(t.Context(), job); err != nil {
		t.Fatal(err)
	}

	if next.calls != 3 {
		t.Fatal("ordinary job did not use its normal workflow", next.calls)
	}
}
