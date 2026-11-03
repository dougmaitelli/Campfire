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
func (s *DeploymentService) Action(ctx context.Context, id string, action model.ServerAction) (model.RuntimeStatus, error) {
	return s.runAction(ctx, id, action, "")
}

// Backup creates a snapshot with a mode chosen for this invocation.
func (s *DeploymentService) Backup(ctx context.Context, id string, mode model.BackupMode) (model.RuntimeStatus, error) {
	return s.runAction(ctx, id, model.ServerActionBackup, string(mode.Normalized()))
}

func (s *DeploymentService) backupOptions(ctx context.Context, id string, mode model.BackupMode) (model.BackupOptions, error) {
	options := model.BackupOptions{Mode: mode.Normalized()}

	server, err := s.Servers.Get(ctx, id)
	if err != nil {
		return options, err
	}

	options.EmberRevision = server.EmberHash

	ember, err := s.Embers.Get(ctx, server.EmberID)
	if err != nil {
		return options, err
	}

	if !server.EmberUpdateAvailable {
		options.Definition = model.NewBackupDefinition(server, ember)
	}

	if options.Mode == model.BackupModeSavedata {
		if server.EmberUpdateAvailable {
			return options, fmt.Errorf("%w: apply the current Ember before using save-data backups", ErrInvalidServer)
		}

		options.Paths = ember.SavedataPaths
	}

	if err := options.Validate(); err != nil {
		return options, fmt.Errorf("%w: %w", ErrInvalidServer, err)
	}

	return options, nil
}

// Reinstall reruns the installer, optionally clearing game files first.
func (s *DeploymentService) Reinstall(ctx context.Context, id string, mode model.ReinstallMode) (model.RuntimeStatus, error) {
	if !mode.Valid() {
		return model.RuntimeStatus{}, ErrInvalidServer
	}

	return s.runAction(ctx, id, model.ServerActionReinstall, string(mode))
}

// Restore restores a named archive on the server's saved execution node.
func (s *DeploymentService) Restore(ctx context.Context, id, name string) (model.RuntimeStatus, error) {
	return s.runAction(ctx, id, model.ServerActionRestore, name)
}

// Console reads bounded logs or sends one line of game console input.
func (s *DeploymentService) Console(ctx context.Context, id, content string, read bool) (model.RuntimeStatus, error) {
	action := model.ServerActionCommand
	if read {
		action = model.ServerActionLogs
	}

	return s.runAction(ctx, id, action, content)
}

func (s *DeploymentService) runAction(ctx context.Context, id string, action model.ServerAction, content string) (model.RuntimeStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	release, err := s.lockServer(ctx, id)
	if err != nil {
		return model.RuntimeStatus{}, err
	}
	defer release()

	return s.runActionUnlocked(ctx, id, action, content)
}

func (s *DeploymentService) runActionUnlocked(ctx context.Context, id string, action model.ServerAction, content string) (model.RuntimeStatus, error) {
	if s.Transfers != nil && action != model.ServerActionStatus && action != model.ServerActionLogs {
		if err := s.Transfers.Store.Guard(ctx, id); err != nil {
			return model.RuntimeStatus{}, err
		}
	}

	if action == model.ServerActionUpdate || action == model.ServerActionReinstall {
		status, err := s.dispatchAction(ctx, id, model.ServerActionStatus, "", "")
		if err != nil {
			return model.RuntimeStatus{}, err
		}

		if status.Busy || (status.State != model.RuntimeStateStopped && status.State != model.RuntimeStateNotInstalled && status.State != model.RuntimeStateDeleted) {
			return model.RuntimeStatus{}, ErrServerActive
		}
	}

	if s.Activity == nil || action == model.ServerActionLogs || action == model.ServerActionStatus || action == model.ServerActionCommand {
		status, err := s.dispatchAction(ctx, id, action, content, "")
		if err == nil && s.Activity != nil && action == model.ServerActionStatus {
			err = s.recordObservation(ctx, id, status)
		}

		return status, err
	}

	if status, err := s.dispatchAction(ctx, id, model.ServerActionStatus, "", ""); err == nil {
		if err := s.recordObservation(ctx, id, status); err != nil {
			return model.RuntimeStatus{}, err
		}

		if status.Busy && (action == model.ServerActionStop || action == model.ServerActionKill) && (status.Operation == model.ServerActionInstall || status.Operation == model.ServerActionReinstall || status.Operation == model.ServerActionUpdate) {
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
		if recordErr := s.Activity.finish(recordCtx, run, model.ExecutionInterrupted, "Dispatch failed; verify server state before retrying."); recordErr != nil {
			return model.RuntimeStatus{}, errors.Join(err, recordErr)
		}

		return model.RuntimeStatus{}, err
	}

	if status.Busy {
		if err := s.Activity.Store.UpdateRun(recordCtx, run.ID, model.ExecutionRunning, "", time.Now()); err != nil {
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

	if run.ServerID == id && run.State == model.ExecutionRunning {
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

	if status, err := s.dispatchAction(ctx, run.ServerID, model.ServerActionStatus, "", ""); err == nil {
		if run.OnlyWhenRunning && status.State != model.RuntimeStateRunning {
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

	content := run.Content
	if run.Action == model.ServerActionBackup {
		content = string(run.BackupMode.Normalized())
	}

	return s.dispatchRequest(ctx, model.RuntimeRequest{ServerID: run.ServerID, Action: run.Action, Content: content, OperationID: run.ID, BackupOptions: model.BackupOptions{StopServer: run.Action == model.ServerActionBackup && run.BackupStopServer}})
}

func (s *DeploymentService) dispatchAction(ctx context.Context, id string, action model.ServerAction, content, operationID string) (model.RuntimeStatus, error) {
	return s.dispatchRequest(ctx, model.RuntimeRequest{ServerID: id, Action: action, Content: content, OperationID: operationID})
}

func (s *DeploymentService) dispatchRequest(ctx context.Context, request model.RuntimeRequest) (model.RuntimeStatus, error) {
	id, action, content := request.ServerID, request.Action, request.Content
	switch action {
	case model.ServerActionInstall, model.ServerActionUpdate, model.ServerActionReinstall, model.ServerActionStart, model.ServerActionStop, model.ServerActionRestart, model.ServerActionKill, model.ServerActionDelete, model.ServerActionStatus, model.ServerActionLogs, model.ServerActionCommand, model.ServerActionBackup, model.ServerActionRestore:
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

	if action == model.ServerActionBackup {
		stopServer := request.BackupOptions.StopServer

		request.BackupOptions, err = s.backupOptions(ctx, id, model.BackupMode(content))
		if err != nil {
			return model.RuntimeStatus{}, err
		}

		request.BackupOptions.StopServer = stopServer
		request.Content = ""
	}

	if action == model.ServerActionRestore && s.Backups != nil {
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

	if server.EmberUpdateAvailable && (action == model.ServerActionInstall || action == model.ServerActionUpdate || action == model.ServerActionReinstall) {
		return model.RuntimeStatus{}, fmt.Errorf("%w: apply the current Ember before installing game files", ErrInvalidServer)
	}

	if !server.EmberUpdateAvailable && action != model.ServerActionStatus && action != model.ServerActionLogs && action != model.ServerActionCommand && action != model.ServerActionStop && action != model.ServerActionKill {
		if _, err := s.sync(ctx, id); err != nil {
			return model.RuntimeStatus{}, err
		}
	}

	if action != model.ServerActionStatus && action != model.ServerActionLogs && action != model.ServerActionCommand {
		if request.OperationID == "" {
			request.OperationID = rand.Text()
		}
	}

	if s.Games != nil {
		if action == model.ServerActionBackup && request.BackupOptions.StopServer {
			s.Games.Invalidate(id)
		}

		switch action {
		case model.ServerActionInstall, model.ServerActionUpdate, model.ServerActionReinstall, model.ServerActionStart, model.ServerActionStop, model.ServerActionRestart, model.ServerActionKill, model.ServerActionDelete, model.ServerActionRestore:
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

	if s.LogAlerts != nil {
		if err := s.LogAlerts.Observe(ctx, node.ID, id, status.LogObservation); err != nil {
			return model.RuntimeStatus{}, err
		}
	}
	// Matching evidence is retrieved through its own permission-checked API.
	if status.LogObservation != nil {
		status.LogMonitorError = status.LogObservation.Error
	}

	status.LogObservation = nil

	if action != model.ServerActionLogs && action != model.ServerActionCommand {
		s.cacheStatus(node.ID, status)
	}

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

		if err := checker.CheckRuntime(server.SparkID, model.RuntimeRequest{ServerID: id, Action: model.ServerActionDelete}); err != nil {
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

	status, err := s.runActionUnlocked(ctx, id, model.ServerActionDelete, "")
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

		status, err = s.runActionUnlocked(ctx, id, model.ServerActionStatus, "")
		if err != nil {
			return err
		}
	}

	if status.Error != "" || status.State != model.RuntimeStateDeleted {
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
