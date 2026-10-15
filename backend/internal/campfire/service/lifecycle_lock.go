package service

import "context"

func (s *DeploymentService) lockServer(ctx context.Context, id string) (func(), error) {
	s.mu.Lock()
	if s.locks == nil {
		s.locks = make(map[string]*deliveryLock)
	}

	lock := s.locks[id]
	if lock == nil {
		lock = &deliveryLock{gate: make(chan struct{}, 1)}
		s.locks[id] = lock
	}

	lock.users++
	s.mu.Unlock()

	drop := func() {
		s.mu.Lock()

		lock.users--
		if lock.users == 0 {
			delete(s.locks, id)
		}
		s.mu.Unlock()
	}

	select {
	case lock.gate <- struct{}{}:
		return func() { <-lock.gate; drop() }, nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}

// BeforeUpdate holds the lifecycle lock through database validation and reservation changes.
func (s *DeploymentService) BeforeUpdate(ctx context.Context, id string) (func(), error) {
	release, err := s.lockServer(ctx, id)
	if err != nil {
		return nil, err
	}

	if s.Transfers != nil {
		if err := s.Transfers.Store.Guard(ctx, id); err != nil {
			release()
			return nil, err
		}
	}

	status, err := s.runActionUnlocked(ctx, id, "status", "")
	if err != nil {
		release()
		return nil, err
	}

	if status.Busy || (status.State != "stopped" && status.State != "not_installed" && status.State != "deleted") {
		release()
		return nil, ErrServerActive
	}

	return release, nil
}
