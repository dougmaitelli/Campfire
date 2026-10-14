package service

import (
	"campfire/internal/model"
	"context"
	"errors"
	"time"
)

// FileDispatcher is shared by in-process storage and authenticated remote delivery.
type FileDispatcher interface {
	Files(context.Context, string, model.FileRequest) (model.FileResult, error)
}

// Files targets the saved placement and coordinates operations with edits and deletion.
func (s *DeploymentService) Files(ctx context.Context, id string, request model.FileRequest) (model.FileResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	release, err := s.lockServer(ctx, id)
	if err != nil {
		return model.FileResult{}, err
	}
	defer release()

	server, err := s.Servers.Get(ctx, id)
	if err != nil {
		return model.FileResult{}, err
	}

	node, err := s.Sparks.Get(ctx, server.SparkID)
	if err != nil {
		return model.FileResult{}, err
	}

	if node.Disabled {
		return model.FileResult{}, ErrDeploymentUnavailable
	}

	dispatch := s.Remote
	if node.Kind == model.SparkLocal {
		dispatch = s.Local
	}

	files, ok := dispatch.(FileDispatcher)
	if !ok {
		return model.FileResult{}, ErrDeploymentUnavailable
	}

	request.ServerID = id

	result, err := files.Files(ctx, node.ID, request)
	if err != nil {
		var failure *model.FileError
		if errors.As(err, &failure) {
			return model.FileResult{}, failure
		}

		return model.FileResult{}, errors.Join(ErrDeploymentUnavailable, err)
	}

	return result, nil
}
