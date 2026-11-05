package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

type copyDocker struct {
	mu     sync.Mutex
	states map[string]string
	calls  [][]string
}

func (d *copyDocker) Run(_ context.Context, _ io.Reader, args ...string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.calls = append(d.calls, append([]string(nil), args...))

	name := args[len(args)-1]
	switch args[0] {
	case "inspect":
		if value := d.states[name]; value != "" {
			return value, nil
		}

		return "no such object", errors.New("missing")
	case "create":
		for i, arg := range args {
			if arg == "--name" {
				d.states[args[i+1]] = "created"
			}
		}
	case "start":
		d.states[name] = "running"
	case "stop", "kill":
		d.states[name] = "exited"
	case "rm":
		delete(d.states, name)
	}

	return "", nil
}

func (r *transferRouter) Connected(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return id != "target" || !r.targetOffline
}

func (r *transferRouter) Files(ctx context.Context, id string, q model.FileRequest) (model.FileResult, error) {
	if !r.Connected(id) {
		return model.FileResult{}, errors.New("destination unavailable")
	}

	return r.stores[id].Files(ctx, id, q)
}

type lostRestoreAck struct {
	*transferRouter
	mu    sync.Mutex
	count int
}

type interruptedCopyOperation struct {
	*transferRouter
	status model.RuntimeStatus
}

func (r *interruptedCopyOperation) Action(ctx context.Context, spark string, q model.RuntimeRequest) (model.RuntimeStatus, error) {
	if q.Action == model.ServerActionStatus {
		return r.status, nil
	}

	return r.transferRouter.Action(ctx, spark, q)
}

func TestCopyInterruptedOperationsStartNewAttempt(t *testing.T) {
	for _, phase := range []model.CopyPhase{model.CopyBackup, model.CopyRestoring} {
		t.Run(string(phase), func(t *testing.T) {
			f, copies, _ := newCopyFixture(t)

			target, err := copies.Create(t.Context(), f.server.ID, model.ServerConfiguration{SparkID: "target"}, 1, true, true, nil)
			if err != nil {
				t.Fatal(err)
			}

			job := copyJob(t, copies, target.ID)
			job = runCopyUntil(t, copies, job.ID, phase)
			operationID, serverID := job.BackupID, job.SourceServerID

			if phase == model.CopyRestoring {
				operationID, serverID = job.RestoreID, job.TargetServerID
			}

			copies.Deployment.Remote = &interruptedCopyOperation{transferRouter: f.router, status: model.RuntimeStatus{ServerID: serverID, State: model.RuntimeStateStopped, OperationID: operationID, OperationState: model.OperationInterrupted}}
			if _, err := copies.step(t.Context(), job); err != nil {
				t.Fatal(err)
			}

			after, err := copies.Store.Get(t.Context(), job.ID)
			if err != nil {
				t.Fatal(err)
			}

			if after.Attempt != job.Attempt+1 || after.BackupID == job.BackupID || after.RestoreID == job.RestoreID || after.Phase != phase {
				t.Fatal("interrupted operation kept its identity or lost its checkpoint", after)
			}

			copies.Deployment.Remote = f.router
			runCopyUntil(t, copies, job.ID, model.CopyComplete)
		})
	}
}

func TestCopyPublicationFailurePausesAndRetryResumesAfterRestart(t *testing.T) {
	f, copies, _ := newCopyFixture(t)

	target, err := copies.Create(t.Context(), f.server.ID, model.ServerConfiguration{SparkID: "target"}, 1, true, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	job := copyJob(t, copies, target.ID)
	job = runCopyUntil(t, copies, job.ID, model.CopyUploading)

	for job.TransferredBytes < job.TotalBytes {
		if _, err := copies.step(t.Context(), job); err != nil {
			t.Fatal(err)
		}

		job, err = copies.Store.Get(t.Context(), job.ID)
		if err != nil {
			t.Fatal(err)
		}
	}

	backups := filepath.Join(f.target.Directory, target.ID, "backups")
	if err := os.MkdirAll(backups, 0700); err != nil {
		t.Fatal(err)
	}

	blocked := filepath.Join(backups, job.ArchiveName())
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}

	if _, err := copies.step(t.Context(), job); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)

	for {
		observed, err := f.router.Job(t.Context(), "target", model.JobRequest{ID: job.ID, ServerID: target.ID, Operation: model.JobOperationStatus})
		if err != nil {
			t.Fatal(err)
		}

		if observed.State == model.JobStateFailed {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("publication did not fail", observed)
		}

		time.Sleep(10 * time.Millisecond)
	}

	// Removing the obstruction must not bypass the failed attempt's checkpoint.
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}

	if _, err := copies.step(t.Context(), job); err == nil {
		t.Fatal("failed publication was silently retried without a new attempt")
	}

	if err := copies.Store.Advance(t.Context(), job.ID, job.Phase, repository.CopyCheckpoint{Phase: model.CopyFailed}); err != nil {
		t.Fatal(err)
	}

	f.target.Close()
	f.target = transferManager(t, f.target.Directory)
	f.target.Docker = &copyDocker{states: make(map[string]string)}
	f.router.stores["target"].Runtime = f.target

	if err := copies.Store.Change(t.Context(), job.ID, false); err != nil {
		t.Fatal(err)
	}

	runCopyUntil(t, copies, job.ID, model.CopyComplete)

	data, err := os.ReadFile(filepath.Join(f.target.Directory, target.ID, "data", "world.bin"))
	if err != nil || !bytes.Equal(data, f.world) {
		t.Fatal("publication retry changed copied data", err)
	}
}

func (r *lostRestoreAck) Action(ctx context.Context, spark string, q model.RuntimeRequest) (model.RuntimeStatus, error) {
	status, err := r.transferRouter.Action(ctx, spark, q)
	if q.Action != model.ServerActionRestore || err != nil {
		return status, err
	}

	r.mu.Lock()
	r.count++
	first := r.count == 1
	r.mu.Unlock()

	if first {
		return status, errors.New("restore acknowledgement lost")
	}

	return status, nil
}

func TestCopyRetryRecognizesAnAlreadySuccessfulRestore(t *testing.T) {
	f, copies, _ := newCopyFixture(t)
	router := &lostRestoreAck{transferRouter: f.router}
	copies.Deployment.Remote = router
	input := f.server.ServerConfiguration
	input.SparkID = "target"

	target, err := copies.Create(t.Context(), f.server.ID, input, 1, true, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	job := copyJob(t, copies, target.ID)
	copies.Start(t.Context())
	t.Cleanup(copies.Close)

	deadline := time.Now().Add(10 * time.Second)

	for {
		job, err = copies.Store.Get(t.Context(), job.ID)
		if err != nil {
			t.Fatal(err)
		}

		status, statusErr := f.router.Action(t.Context(), "target", model.RuntimeRequest{ServerID: target.ID, Action: model.ServerActionStatus})
		if statusErr == nil && job.Phase == model.CopyFailed && status.OperationState == model.OperationSucceeded && !status.Busy {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("restore did not finish after acknowledgement loss", job, status, statusErr)
		}

		time.Sleep(10 * time.Millisecond)
	}

	if err := copies.Store.Change(t.Context(), job.ID, false); err != nil {
		t.Fatal(err)
	}

	for {
		job, err = copies.Store.Get(t.Context(), job.ID)
		if err != nil {
			t.Fatal(err)
		}

		if job.Phase == model.CopyComplete {
			break
		}

		if job.Phase == model.CopyFailed || time.Now().After(deadline) {
			t.Fatal("retry did not reconcile successful restore", job)
		}

		time.Sleep(10 * time.Millisecond)
	}

	router.mu.Lock()
	defer router.mu.Unlock()

	if router.count != 1 {
		t.Fatal("retry repeated an already successful restore", router.count)
	}
}

func newCopyFixture(t *testing.T) (*transferFixture, *CopyService, *copyDocker) {
	t.Helper()

	f := newTransferFixture(t)
	if err := f.db.ORM().Model(&database.Spark{}).Where("id IN ?", []string{"source", "target"}).Update("memory_mb", 1024).Error; err != nil {
		t.Fatal(err)
	}

	docker := &copyDocker{states: map[string]string{"campfire-" + f.server.ID: "exited"}}
	f.source.Docker = docker
	f.target.Docker = &copyDocker{states: make(map[string]string)}
	d := f.service.Deployment
	d.Sparks = NewSparkService(repository.NewSparkRepository(f.db.ORM()), f.router)
	d.Backups = NewBackupService(repository.NewBackupRepository(f.db.ORM()), d, nil)
	s := NewCopyService(repository.NewCopyRepository(f.db.ORM()), d)
	d.Copies = s

	return f, s, docker
}

func copyJob(t *testing.T, s *CopyService, serverID string) model.CopyWork {
	t.Helper()

	jobs, err := s.Store.Jobs().List(t.Context(), serverID, []model.JobType{model.JobCopy}, nil)
	if err != nil || len(jobs) != 1 {
		t.Fatal("missing copy job", jobs, err)
	}

	work, err := s.Store.Get(t.Context(), jobs[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	return work
}

func runCopyUntil(t *testing.T, s *CopyService, id string, phase model.CopyPhase) model.CopyWork {
	t.Helper()
	claimJob(t, s.Store.Jobs(), id)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		job, err := s.Store.Get(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}

		if job.Phase == phase {
			return job
		}

		if _, err := s.step(t.Context(), job); err != nil {
			t.Fatalf("copy failed in %s: %v", job.Phase, err)
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatal("copy did not reach", phase)

	return model.CopyWork{}
}

func TestCopyRunningSourcePreservesFilesAndState(t *testing.T) {
	for _, target := range []string{"source", "target"} {
		t.Run(target, func(t *testing.T) {
			f, s, docker := newCopyFixture(t)
			docker.states["campfire-"+f.server.ID] = "running"

			policy := model.BackupPolicy{Storage: model.BackupStorageLocal, LocalKeepLast: 3}
			if err := s.Deployment.Backups.SavePolicy(t.Context(), f.server.ID, policy); err != nil {
				t.Fatal(err)
			}

			ownerCalls := 0

			manager := f.target
			if target == "source" {
				manager = f.source
			}

			manager.Ownership = func(_ string, user string) error {
				if user != "1000:1000" {
					return errors.New("unexpected owner")
				}

				ownerCalls++

				return nil
			}

			saved, err := s.Create(t.Context(), f.server.ID, model.ServerConfiguration{SparkID: target}, 2, true, true, nil)
			if err != nil {
				t.Fatal(err)
			}

			if saved.ID == f.server.ID || saved.Name != "World (copy)" || saved.OwnerUserID != 2 || saved.Ports[0].ContainerPort != f.server.Ports[0].ContainerPort {
				t.Fatal("incorrect copy configuration", saved)
			}

			job := copyJob(t, s, saved.ID)
			if _, err := s.Deployment.Action(t.Context(), saved.ID, model.ServerActionStart); !errors.Is(err, repository.ErrJobActive) {
				t.Fatal("unfinished copy could start", err)
			}

			if err := s.Deployment.Servers.Delete(t.Context(), f.server.ID); !errors.Is(err, repository.ErrJobActive) {
				t.Fatal("source was not protected", err)
			}

			runCopyUntil(t, s, job.ID, model.CopyComplete)
			worldPath := filepath.Join(manager.Directory, saved.ID, "data", "world.bin")

			data, err := os.ReadFile(worldPath)
			if err != nil || !bytes.Equal(data, f.world) || ownerCalls == 0 {
				t.Fatal("world or ownership was not copied", err)
			}

			if err := os.WriteFile(worldPath, []byte("independent destination"), 0600); err != nil {
				t.Fatal(err)
			}

			source, err := os.ReadFile(filepath.Join(f.source.Directory, f.server.ID, "data", "world.bin"))
			if err != nil || !bytes.Equal(source, f.world) {
				t.Fatal("copy changed source files", err)
			}

			if _, err := os.Stat(filepath.Join(manager.Directory, saved.ID, "backups", "nightly.tar.gz")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("historical backup was copied", err)
			}

			originalBackup, err := os.ReadFile(filepath.Join(f.source.Directory, f.server.ID, "backups", "nightly.tar.gz"))
			if err != nil || string(originalBackup) != "existing completed backup" {
				t.Fatal("source backup was removed", err)
			}

			copiedPolicy, err := s.Store.Policy(t.Context(), saved.ID)
			if err != nil || copiedPolicy.LocalKeepLast != 3 {
				t.Fatal("backup policy was not copied", err)
			}

			for _, pair := range []struct{ id, state string }{{f.server.ID, "running"}, {saved.ID, "stopped"}} {
				status, err := s.Deployment.Action(t.Context(), pair.id, model.ServerActionStatus)
				if err != nil || string(status.State) != pair.state {
					t.Fatal("incorrect final state", pair, status, err)
				}
			}

			docker.mu.Lock()
			defer docker.mu.Unlock()

			for _, call := range docker.calls {
				if (call[0] == "stop" || call[0] == "kill" || call[0] == "pause") && call[len(call)-1] == "campfire-"+f.server.ID {
					t.Fatal("copy interrupted source", call)
				}

				if strings.Contains(strings.Join(call, " "), "-install") {
					t.Fatal("copy ran installer", call)
				}
			}
		})
	}
}

func TestCopySettingsOfflineAndReservations(t *testing.T) {
	f, s, _ := newCopyFixture(t)
	f.router.targetOffline = true
	configuration := f.server.ServerConfiguration
	configuration.Name, configuration.SparkID, configuration.Ports = "Settings copy", "target", f.ports

	saved, err := s.Create(t.Context(), f.server.ID, configuration, 2, false, false, nil)
	if err != nil || saved.Status != model.RuntimeStateNotInstalled {
		t.Fatal("offline settings copy failed", saved, err)
	}

	jobs, err := s.Store.Jobs().List(t.Context(), saved.ID, []model.JobType{model.JobCopy}, nil)
	if err != nil || len(jobs) != 0 {
		t.Fatal("settings copy created a file job", jobs, err)
	}

	if _, err := s.Create(t.Context(), f.server.ID, configuration, 2, false, false, nil); !errors.Is(err, ErrServerConflict) {
		t.Fatal("duplicate name/ports were accepted", err)
	}
}

func TestCopyCancellationRemovesOnlyUnfinishedDestination(t *testing.T) {
	f, s, _ := newCopyFixture(t)

	saved, err := s.Create(t.Context(), f.server.ID, model.ServerConfiguration{SparkID: "target"}, 1, true, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	job := copyJob(t, s, saved.ID)
	runCopyUntil(t, s, job.ID, model.CopyUploading)

	if err := s.Store.Change(t.Context(), job.ID, true); err != nil {
		t.Fatal(err)
	}

	runCopyUntil(t, s, job.ID, model.CopyCancelled)

	if _, err := s.Deployment.Servers.Get(t.Context(), saved.ID); !errors.Is(err, ErrServerNotFound) {
		t.Fatal("cancelled destination remains", err)
	}

	if _, err := os.Stat(filepath.Join(f.source.Directory, f.server.ID, "data", "world.bin")); err != nil {
		t.Fatal("cancel deleted source", err)
	}

	if err := s.Store.Guard(t.Context(), f.server.ID); err != nil {
		t.Fatal("cancelled job kept source locked", err)
	}
}

func TestCopyCanCancelBeforePreparingAndRetriesAfterOfflineCleanup(t *testing.T) {
	f, s, _ := newCopyFixture(t)

	saved, err := s.Create(t.Context(), f.server.ID, model.ServerConfiguration{SparkID: "target"}, 1, true, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	job := copyJob(t, s, saved.ID)

	job = runCopyUntil(t, s, job.ID, model.CopyBackup)
	if err := s.Store.Change(t.Context(), job.ID, true); err != nil {
		t.Fatal(err)
	}

	job, err = s.Store.Get(t.Context(), job.ID)
	if err != nil {
		t.Fatal(err)
	}

	f.router.targetOffline = true

	if _, err := s.step(t.Context(), job); err == nil {
		t.Fatal("offline destination was silently discarded")
	}

	if err := s.Store.Guard(t.Context(), saved.ID); !errors.Is(err, repository.ErrJobActive) {
		t.Fatal("offline cleanup released destination", err)
	}

	f.router.targetOffline = false

	runCopyUntil(t, s, job.ID, model.CopyCancelled)

	if _, err := s.Deployment.Servers.Get(t.Context(), saved.ID); !errors.Is(err, ErrServerNotFound) {
		t.Fatal("queued destination remains", err)
	}
}

func TestCopyFailureRetainsReservationsAndRetryRestores(t *testing.T) {
	f, s, _ := newCopyFixture(t)

	saved, err := s.Create(t.Context(), f.server.ID, model.ServerConfiguration{SparkID: "target"}, 1, true, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	job := copyJob(t, s, saved.ID)
	runCopyUntil(t, s, job.ID, model.CopyRestoring)

	f.target.Ownership = func(string, string) error { return errors.New("owner unavailable") }

	s.Start(t.Context())

	deadline := time.Now().Add(5 * time.Second)

	for {
		job, err = s.Store.Get(t.Context(), job.ID)
		if err != nil {
			s.Close()
			t.Fatal(err)
		}

		if job.Phase == model.CopyFailed {
			break
		}

		if time.Now().After(deadline) {
			s.Close()
			t.Fatal("failed restore was not retained")
		}

		time.Sleep(10 * time.Millisecond)
	}

	s.Close()

	checkpoint, err := s.Store.Jobs().Get(t.Context(), job.ID)
	if err != nil {
		t.Fatal(err)
	}

	if !job.Active || checkpoint.Step != model.JobImporting {
		t.Fatal("failed restore lost its checkpoint", job)
	}

	if _, err := s.Deployment.Action(t.Context(), saved.ID, model.ServerActionStart); !errors.Is(err, repository.ErrJobActive) {
		t.Fatal("failed destination could start", err)
	}

	f.target.Ownership = func(string, string) error { return nil }

	if err := s.Store.Change(t.Context(), job.ID, false); err != nil {
		t.Fatal(err)
	}

	runCopyUntil(t, s, job.ID, model.CopyComplete)
}

func TestCopyUploadRecoversAfterSparkAndPanelRestart(t *testing.T) {
	f, s, _ := newCopyFixture(t)

	saved, err := s.Create(t.Context(), f.server.ID, model.ServerConfiguration{SparkID: "target"}, 1, true, true, nil)
	if err != nil {
		t.Fatal(err)
	}

	job := copyJob(t, s, saved.ID)

	job = runCopyUntil(t, s, job.ID, model.CopyUploading)
	if _, err := s.step(t.Context(), job); err != nil {
		t.Fatal(err)
	}

	job, err = s.Store.Get(t.Context(), job.ID)
	if err != nil || job.TransferredBytes == 0 {
		t.Fatal("upload checkpoint missing", job, err)
	}

	f.target.Close()
	f.target = transferManager(t, f.target.Directory)
	f.target.Docker = &copyDocker{states: make(map[string]string)}
	f.router.stores["target"].Runtime = f.target
	s = NewCopyService(repository.NewCopyRepository(f.db.ORM()), s.Deployment)
	s.Deployment.Copies = s
	runCopyUntil(t, s, job.ID, model.CopyComplete)

	data, err := os.ReadFile(filepath.Join(f.target.Directory, saved.ID, "data", "world.bin"))
	if err != nil || !bytes.Equal(data, f.world) {
		t.Fatal("restart recovery changed world", err)
	}
}
