package service

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"campfire/internal/model"
)

// RuntimeDispatcher invokes the same runtime locally or over a Spark session.
type RuntimeDispatcher interface {
	Action(context.Context, string, model.RuntimeRequest) (model.RuntimeStatus, error)
}

// Action delivers current configuration before submitting a lifecycle operation.
func (s *DeploymentService) Action(ctx context.Context, id, action string) (model.RuntimeStatus, error) {
	return s.runAction(ctx, id, action, "")
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
	switch action {
	case "install", "start", "stop", "restart", "kill", "delete", "status", "logs", "command":
	default:
		return model.RuntimeStatus{}, ErrInvalidServer
	}

	if action != "status" && action != "logs" && action != "command" {
		if _, err := s.sync(ctx, id); err != nil {
			return model.RuntimeStatus{}, err
		}
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
	if node.Kind == model.SparkLocal {
		dispatch = s.Local
	}

	runtime, ok := dispatch.(RuntimeDispatcher)
	if !ok {
		return model.RuntimeStatus{}, ErrDeploymentUnavailable
	}

	request := model.RuntimeRequest{ServerID: id, Action: action, Content: content}
	if action != "status" && action != "logs" && action != "command" {
		request.OperationID = rand.Text()
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
func (s *DeploymentService) CachedState(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if status, ok := s.statuses[id]; ok && time.Since(s.observed[id]) < 10*time.Second {
		return status.State
	}

	return "unknown"
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

	if err := s.Servers.Delete(ctx, id); err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.statuses, id)
	delete(s.observed, id)
	s.mu.Unlock()

	return nil
}
