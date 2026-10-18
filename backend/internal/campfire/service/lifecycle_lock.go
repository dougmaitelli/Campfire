package service

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"campfire/internal/shared/model"
)

// BeforeEdit allows saved configuration changes without disturbing the running
// process. Reservations already in use must remain held until the server stops.
func (s *DeploymentService) BeforeEdit(ctx context.Context, id string, input model.ServerConfiguration) (func(), error) {
	release, err := s.lockServer(ctx, id)
	if err != nil {
		return nil, err
	}

	server, err := s.Servers.Get(ctx, id)
	if err != nil {
		release()
		return nil, err
	}
	// Names and descriptions never alter the runtime or its reservations.
	if model.SameRuntimeConfiguration(server.ServerConfiguration, input) {
		return release, nil
	}

	if s.Transfers != nil {
		if err := s.Transfers.Store.Guard(ctx, id); err != nil {
			release()
			return nil, err
		}
	}

	status, err := s.runActionUnlocked(ctx, id, "status", "")
	if err != nil {
		if !errors.Is(err, ErrDeploymentUnavailable) || ctx.Err() != nil {
			release()
			return nil, err
		}

		status.State = "unknown"
	}

	if status.Busy || (status.State != "unknown" && status.State != "unavailable" && status.State != "running" && status.State != "stopped" && status.State != "not_installed" && status.State != "deleted") {
		release()
		return nil, ErrServerActive
	}

	if status.State == "running" || status.State == "unknown" || status.State == "unavailable" {
		server, err := s.Servers.Get(ctx, id)
		if err != nil {
			release()
			return nil, err
		}

		if input.CPULimit < server.CPULimit || input.MemoryMB < server.MemoryMB || input.DiskMB < server.DiskMB {
			release()
			return nil, fmt.Errorf("%w: stop the server before reducing resource reservations", ErrServerActive)
		}

		for _, used := range server.Ports {
			found := false

			for _, port := range input.Ports {
				oldIP, oldErr := netip.ParseAddr(used.BindIP)

				newIP, newErr := netip.ParseAddr(port.BindIP)
				if oldErr == nil && newErr == nil && oldIP.Unmap() == newIP.Unmap() && used.HostPort == port.HostPort && used.Protocol == port.Protocol {
					found = true
					break
				}
			}

			if !found {
				release()
				return nil, fmt.Errorf("%w: stop the server before removing or replacing reserved host ports", ErrServerActive)
			}
		}
	}

	return release, nil
}

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
