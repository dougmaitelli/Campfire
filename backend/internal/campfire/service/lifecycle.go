package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"

	"campfire/internal/shared/model"
)

// RuntimeDispatcher invokes the runtime over a Spark session.
type RuntimeDispatcher interface {
	Action(context.Context, string, model.RuntimeRequest) (model.RuntimeStatus, error)
}

// Action delivers current configuration before submitting a lifecycle operation.
func (s *DeploymentService) Action(ctx context.Context, id, action string) (model.RuntimeStatus, error) {
	return s.runAction(ctx, id, action, "")
}

// Reinstall reruns the installer, optionally clearing game files first.
func (s *DeploymentService) Reinstall(ctx context.Context, id, mode string) (model.RuntimeStatus, error) {
	if mode != "" && mode != "keep" && mode != "clean" {
		return model.RuntimeStatus{}, ErrInvalidServer
	}

	return s.runAction(ctx, id, "reinstall", mode)
}

// Restore restores a named archive on the server's saved execution node.
func (s *DeploymentService) Restore(ctx context.Context, id, name string) (model.RuntimeStatus, error) {
	return s.runAction(ctx, id, "restore", name)
}

// Console reads bounded logs or sends one line of game console input.
func (s *DeploymentService) Console(ctx context.Context, id, content string, read bool) (model.RuntimeStatus, error) {
	action := "command"
	if read {
		action = "logs"
	}

	return s.runAction(ctx, id, action, content)
}

func (s *DeploymentService) runAction(ctx context.Context, id, action, content string) (model.RuntimeStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	release, err := s.lockServer(ctx, id)
	if err != nil {
		return model.RuntimeStatus{}, err
	}
	defer release()

	return s.runActionUnlocked(ctx, id, action, content)
}

func (s *DeploymentService) runActionUnlocked(ctx context.Context, id, action, content string) (model.RuntimeStatus, error) {
	if s.Transfers != nil && action != "status" && action != "logs" {
		if err := s.Transfers.Store.Guard(ctx, id); err != nil {
			return model.RuntimeStatus{}, err
		}
	}

	if action == "update" || action == "reinstall" {
		status, err := s.dispatchAction(ctx, id, "status", "", "")
		if err != nil {
			return model.RuntimeStatus{}, err
		}

		if status.Busy || (status.State != "stopped" && status.State != "not_installed" && status.State != "deleted") {
			return model.RuntimeStatus{}, ErrServerActive
		}
	}

	if s.Activity == nil || action == "logs" || action == "status" || action == "command" {
		status, err := s.dispatchAction(ctx, id, action, content, "")
		if err == nil && s.Activity != nil && action == "status" {
			err = s.recordObservation(ctx, id, status)
		}

		return status, err
	}

	if status, err := s.dispatchAction(ctx, id, "status", "", ""); err == nil {
		if err := s.recordObservation(ctx, id, status); err != nil {
			return model.RuntimeStatus{}, err
		}

		if status.Busy && (action == "stop" || action == "kill") && (status.Operation == "install" || status.Operation == "reinstall" || status.Operation == "update") {
			status, err := s.dispatchAction(ctx, id, action, content, "")
			if err == nil {
				err = s.recordObservation(ctx, id, status)
			}

			return status, err
		}
	}

	run, err := s.Activity.BeginManual(ctx, id, action, content)
	if err != nil {
		return model.RuntimeStatus{}, err
	}

	status, err := s.dispatchAction(ctx, id, action, content, run.ID)

	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()

	if err != nil {
		if recordErr := s.Activity.finish(recordCtx, run, "interrupted", "Dispatch failed; verify server state before retrying."); recordErr != nil {
			return model.RuntimeStatus{}, errors.Join(err, recordErr)
		}

		return model.RuntimeStatus{}, err
	}

	if status.Busy {
		if err := s.Activity.Store.UpdateRun(recordCtx, run.ID, "running", "", time.Now()); err != nil {
			return model.RuntimeStatus{}, err
		}
	}

	if err := s.Activity.observe(recordCtx, run, status); err != nil {
		return model.RuntimeStatus{}, err
	}

	return status, nil
}

func (s *DeploymentService) recordObservation(ctx context.Context, id string, status model.RuntimeStatus) error {
	if status.OperationID == "" {
		return nil
	}

	run, err := s.Activity.Store.Run(ctx, status.OperationID)
	if errors.Is(err, repository.ErrAccessNotFound) {
		return nil
	}

	if err != nil {
		return err
	}

	if run.ServerID == id && run.State == "running" {
		return s.Activity.observe(ctx, run, status)
	}

	return nil
}

func (s *DeploymentService) executeTracked(ctx context.Context, run database.ScheduleRun) (model.RuntimeStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	release, err := s.lockServer(ctx, run.ServerID)
	if err != nil {
		return model.RuntimeStatus{}, err
	}
	defer release()

	if _, err := s.Activity.checkExecution(ctx, run); err != nil {
		return model.RuntimeStatus{}, err
	}

	if status, err := s.dispatchAction(ctx, run.ServerID, "status", "", ""); err == nil {
		if run.OnlyWhenRunning && status.State != "running" {
			return model.RuntimeStatus{}, errServerNotRunning
		}

		if err := s.recordObservation(ctx, run.ServerID, status); err != nil {
			return model.RuntimeStatus{}, err
		}

		if status.Busy {
			return model.RuntimeStatus{}, ErrServerActive
		}
	} else if run.OnlyWhenRunning {
		return model.RuntimeStatus{}, err
	}

	return s.dispatchAction(ctx, run.ServerID, run.Action, run.Content, run.ID)
}

func (s *DeploymentService) dispatchAction(ctx context.Context, id, action, content, operationID string) (model.RuntimeStatus, error) {
	switch action {
	case "install", "update", "reinstall", "start", "stop", "restart", "kill", "delete", "status", "logs", "command", "backup", "restore":
	default:
		return model.RuntimeStatus{}, ErrInvalidServer
	}

	server, err := s.Servers.Get(ctx, id)
	if err != nil {
		return model.RuntimeStatus{}, err
	}

	node, err := s.Sparks.Get(ctx, server.SparkID)
	if err != nil {
		return model.RuntimeStatus{}, err
	}

	if node.Disabled {
		return model.RuntimeStatus{}, ErrDeploymentUnavailable
	}

	dispatch := s.Remote

	runtime, ok := dispatch.(RuntimeDispatcher)
	if !ok {
		return model.RuntimeStatus{}, ErrDeploymentUnavailable
	}

	request := model.RuntimeRequest{ServerID: id, Action: model.ServerAction(action), Content: content}
	if action == "restore" && s.Backups != nil {
		request.BackupSource, err = s.Backups.Source(ctx, id, content)
		if err != nil {
			return model.RuntimeStatus{}, err
		}
	}

	if checker, ok := dispatch.(interface {
		CheckRuntime(string, model.RuntimeRequest) error
	}); ok {
		if err := checker.CheckRuntime(node.ID, request); err != nil {
			return model.RuntimeStatus{}, errors.Join(ErrDeploymentUnavailable, err)
		}
	}

	if server.EmberUpdateAvailable && (action == "install" || action == "update" || action == "reinstall") {
		return model.RuntimeStatus{}, fmt.Errorf("%w: apply the current Ember before installing game files", ErrInvalidServer)
	}

	if !server.EmberUpdateAvailable && action != "status" && action != "logs" && action != "command" && action != "stop" && action != "kill" {
		if _, err := s.sync(ctx, id); err != nil {
			return model.RuntimeStatus{}, err
		}
	}

	if action != "status" && action != "logs" && action != "command" {
		request.OperationID = operationID
		if request.OperationID == "" {
			request.OperationID = rand.Text()
		}
	}

	if s.Games != nil {
		switch action {
		case "install", "update", "reinstall", "start", "stop", "restart", "kill", "delete", "restore":
			s.Games.Invalidate(id)
		}
	}

	status, err := runtime.Action(ctx, node.ID, request)
	if err != nil {
		s.mu.Lock()
		delete(s.statuses, id)
		s.mu.Unlock()

		return model.RuntimeStatus{}, errors.Join(ErrDeploymentUnavailable, err)
	}

	s.mu.Lock()
	if s.statuses == nil {
		s.statuses = make(map[string]model.RuntimeStatus)
		s.observed = make(map[string]time.Time)
	}

	if action == "status" || status.Busy {
		s.statuses[id] = status
		s.observed[id] = time.Now()
	}
	s.mu.Unlock()

	return status, nil
}

// CachedState supplies the latest observed state without blocking collection requests.
func (s *DeploymentService) CachedState(id string) model.RuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()

	if status, ok := s.statuses[id]; ok && time.Since(s.observed[id]) < 10*time.Second {
		return status.State
	}

	return model.RuntimeStateUnknown
}

// Delete removes the node runtime before releasing resource and port reservations.
func (s *DeploymentService) Delete(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	release, lockErr := s.lockServer(ctx, id)
	if lockErr != nil {
		return lockErr
	}
	defer release()

	if s.Transfers != nil {
		if err := s.Transfers.Store.Guard(ctx, id); err != nil {
			return err
		}
	}

	if checker, ok := s.Remote.(interface {
		CheckRuntime(string, model.RuntimeRequest) error
	}); ok {
		server, err := s.Servers.Get(ctx, id)
		if err != nil {
			return err
		}

		if err := checker.CheckRuntime(server.SparkID, model.RuntimeRequest{ServerID: id, Action: "delete"}); err != nil {
			return errors.Join(ErrDeploymentUnavailable, err)
		}
	}

	if s.Backups != nil {
		releaseBackups, err := s.Backups.BeforeServerDelete(ctx, id)
		if err != nil {
			return err
		}
		defer releaseBackups()
	}

	status, err := s.runActionUnlocked(ctx, id, "delete", "")
	if err != nil {
		return err
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for status.Busy {
		select {
		case <-ctx.Done():
			return errors.Join(ErrDeploymentUnavailable, ctx.Err())
		case <-ticker.C:
		}

		status, err = s.runActionUnlocked(ctx, id, "status", "")
		if err != nil {
			return err
		}
	}

	if status.Error != "" || status.State != "deleted" {
		return ErrDeploymentUnavailable
	}

	if s.Backups != nil {
		if err := s.Backups.ForgetServer(ctx, id); err != nil {
			return err
		}
	}

	if err := s.Servers.Delete(ctx, id); err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.statuses, id)
	delete(s.observed, id)
	s.mu.Unlock()

	return nil
}
