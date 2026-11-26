//go:build !production

package devmock

import (
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func TestMockQuickCopyPreservesRunningSource(t *testing.T) {
	runtime, db, _, servers := fixture(t)
	source := onlineServer(t, runtime, servers)
	transfers := mockTransferService(t, runtime, db)
	deployment := transfers.Deployment
	deployment.Backups = service.NewBackupService(repository.NewBackupRepository(db.ORM()), deployment, nil)
	copies := service.NewCopyService(repository.NewCopyRepository(db.ORM()), deployment)
	deployment.Copies = copies

	target, err := copies.Create(t.Context(), source.ID, model.ServerConfiguration{SparkID: source.SparkID}, source.OwnerUserID, true, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	copies.Start(t.Context())
	t.Cleanup(copies.Close)

	deadline := time.Now().Add(10 * time.Second)

	for {
		jobs, err := copies.Store.Jobs().List(t.Context(), target.ID, []model.JobType{model.JobCopy}, nil)
		if err != nil || len(jobs) != 1 {
			t.Fatal(jobs, err)
		}

		if jobs[0].Status == model.JobCompleted {
			break
		}

		if jobs[0].Status == model.JobFailed || time.Now().After(deadline) {
			t.Fatal("mock copy did not complete", jobs[0].Status, jobs[0].Error)
		}

		time.Sleep(10 * time.Millisecond)
	}

	for _, check := range []struct {
		id    string
		state model.RuntimeState
	}{{source.ID, model.RuntimeStateRunning}, {target.ID, model.RuntimeStateStopped}} {
		status, err := deployment.Action(t.Context(), check.id, model.ServerActionStatus)
		if err != nil || status.State != check.state {
			t.Fatal(check.id, status.State, err)
		}
	}
}
