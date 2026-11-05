package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// JobHandler owns workflow decisions; the runner owns dispatch and recovery.
// Step must checkpoint with the supplied job as its compare-and-swap expectation.
type JobHandler interface {
	Ready(database.ServerJob) bool
	Step(context.Context, database.ServerJob) (bool, error)
	Cancel(context.Context, database.ServerJob) (bool, error)
}

type workflowHandler struct {
	deployment *DeploymentService
	typeID     model.JobType
}

type jobStepError struct {
	err  error
	step model.JobStep
}

func (e jobStepError) Error() string { return e.err.Error() }
func (e jobStepError) Unwrap() error { return e.err }

// JobWaitingFor reports live connection requirements without changing the
// persisted checkpoint or counting offline time against execution limits.
func (s *DeploymentService) JobWaitingFor(job database.ServerJob) []string {
	waiting := make([]string, 0, 2)
	if !job.Active() || job.Status == model.JobFailed {
		return waiting
	}

	sourceOnly := job.Type == model.JobTransfer && job.Status == model.JobCancelling && (job.Step == model.JobPreparing || job.Step == model.JobExporting)

	ids := []string{job.SourceSparkID}
	if job.SourceSparkID != job.TargetSparkID && !sourceOnly {
		ids = append(ids, job.TargetSparkID)
	}

	for _, id := range ids {
		if s.Sparks.connectivity == nil || !s.Sparks.connectivity.Connected(id) {
			waiting = append(waiting, id)
		}
	}

	return waiting
}

func (h workflowHandler) Ready(job database.ServerJob) bool {
	// Cancellation may be able to clean an online endpoint while another is
	// offline. A never-started job is cancelled atomically by the repository.
	return job.Status == model.JobCancelling || len(h.deployment.JobWaitingFor(job)) == 0
}

func (h workflowHandler) Step(ctx context.Context, job database.ServerJob) (bool, error) {
	ctx = repository.WithJobCheckpoint(ctx, job)
	if h.typeID == model.JobCopy {
		work, err := h.deployment.Copies.Store.Get(ctx, job.ID)
		if err != nil {
			return false, err
		}

		return h.deployment.Copies.step(ctx, work)
	}

	work, err := h.deployment.Transfers.Store.Get(ctx, job.ID)
	if err != nil {
		return false, err
	}

	wait, err := h.deployment.Transfers.step(ctx, work)
	if errors.Is(err, errTransferCorrupt) {
		return wait, jobStepError{err, model.JobCopying}
	}

	return wait, err
}

func (h workflowHandler) Cancel(ctx context.Context, job database.ServerJob) (bool, error) {
	return h.Step(ctx, job)
}

// StartJobs starts the shared coordinator after both workflow handlers are wired.
func (s *DeploymentService) StartJobs(ctx context.Context) {
	if s.Copies != nil {
		s.startJobs(ctx, s.Copies.Store.Jobs())
		return
	}

	if s.Transfers != nil {
		s.startJobs(ctx, s.Transfers.Store.Jobs())
	}
}

// CloseJobs preserves checkpoints and waits for all job workers to stop.
func (s *DeploymentService) CloseJobs() { s.closeJobs() }

func (s *DeploymentService) startJobs(ctx context.Context, store *repository.JobRepository) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Jobs != nil {
		return
	}

	handlers := make(map[model.JobType]JobHandler)
	if s.Copies != nil {
		handlers[model.JobCopy] = workflowHandler{s, model.JobCopy}
	}

	if s.Transfers != nil {
		handlers[model.JobTransfer] = workflowHandler{s, model.JobTransfer}
	}

	if wrapper, ok := s.Remote.(interface{ WrapJobHandler(JobHandler) JobHandler }); ok {
		for kind, handler := range handlers {
			handlers[kind] = wrapper.WrapJobHandler(handler)
		}
	}

	s.Jobs = NewJobService(store, handlers)
	s.Jobs.Start(ctx)
}

func (s *DeploymentService) closeJobs() {
	s.mu.Lock()
	jobs := s.Jobs
	s.mu.Unlock()

	if jobs != nil {
		jobs.Close()
	}

	s.mu.Lock()
	if s.Jobs == jobs {
		s.Jobs = nil
	}
	s.mu.Unlock()
}

// JobService runs bounded, recoverable workflows through registered handlers.
type JobService struct {
	Store    *repository.JobRepository
	handlers map[model.JobType]JobHandler
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	running  map[string]database.ServerJob
	wg       sync.WaitGroup
}

// NewJobService constructs the shared background coordinator.
func NewJobService(store *repository.JobRepository, handlers map[model.JobType]JobHandler) *JobService {
	return &JobService{Store: store, handlers: handlers, running: make(map[string]database.ServerJob)}
}

// Start resumes pending work independently of browser connections.
func (s *JobService) Start(ctx context.Context) {
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			s.pump()

			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// Close stops dispatch and waits for workers without discarding checkpoints.
func (s *JobService) Close() {
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
}

func (s *JobService) pump() {
	jobs, err := s.Store.Runnable(s.ctx)
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, job := range jobs {
		if len(s.running) >= 4 || s.ctx.Err() != nil {
			return
		}

		if _, exists := s.running[job.ID]; exists {
			continue
		}

		if handler := s.handlers[job.Type]; handler != nil && !handler.Ready(job) {
			continue
		}

		counts := make(map[string]int)
		for _, active := range s.running {
			counts[active.SourceSparkID]++
			if active.TargetSparkID != active.SourceSparkID {
				counts[active.TargetSparkID]++
			}
		}

		if counts[job.SourceSparkID] >= 2 || counts[job.TargetSparkID] >= 2 {
			continue
		}

		s.running[job.ID] = job

		s.wg.Add(1)
		go s.run(job.ID)
	}
}

func (s *JobService) run(id string) {
	defer s.wg.Done()
	defer func() { s.mu.Lock(); delete(s.running, id); s.mu.Unlock() }()

	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Minute)
	defer cancel()

	for {
		job, err := s.Store.Get(ctx, id)
		if err != nil && ctx.Err() != nil {
			s.expire(ctx, id)

			return
		}

		if err != nil || !job.Active() || job.Status == model.JobFailed {
			return
		}

		handler := s.handlers[job.Type]
		if handler != nil && !handler.Ready(job) {
			return
		}

		if job.Status == model.JobPending {
			if err := s.Store.Advance(ctx, job, map[string]any{"status": model.JobRunning}); err != nil && !errors.Is(err, repository.ErrJobState) {
				return
			}

			continue
		}

		wait := false

		if handler == nil {
			err = fmt.Errorf("unsupported job type %q", job.Type)
		} else if job.Status == model.JobCancelling {
			wait, err = handler.Cancel(ctx, job)
		} else {
			wait, err = handler.Step(ctx, job)
		}

		if errors.Is(err, repository.ErrJobState) {
			continue
		}

		if err != nil && handler != nil && !handler.Ready(job) {
			return
		}

		if err != nil || ctx.Err() != nil {
			if s.ctx.Err() != nil {
				return
			}

			if err == nil {
				err = ctx.Err()
			}

			record, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			// Preserve cancellation intent so delayed cleanup never becomes runnable
			// as an ordinary workflow step after a connection failure.
			status := model.JobFailed
			if job.Status == model.JobCancelling {
				status = model.JobCancelling
			}

			changes := map[string]any{"status": status, "error": fmt.Sprintf("%.1024s", err.Error())}

			var failure jobStepError
			if errors.As(err, &failure) {
				changes["step"] = failure.step
			}

			_ = s.Store.Advance(record, job, changes)

			stop()

			return
		}

		if wait {
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				s.expire(ctx, id)

				return
			case <-timer.C:
			}
		}
	}
}

func (s *JobService) expire(ctx context.Context, id string) {
	if s.ctx.Err() != nil {
		return
	}

	record, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()

	job, err := s.Store.Get(record, id)
	if err != nil || !job.Active() || job.Status == model.JobFailed {
		return
	}

	status := model.JobFailed
	if job.Status == model.JobCancelling {
		status = model.JobCancelling
	}

	_ = s.Store.Advance(record, job, map[string]any{"status": status, "error": "Job execution timed out; retry to continue from the saved checkpoint."})
}
