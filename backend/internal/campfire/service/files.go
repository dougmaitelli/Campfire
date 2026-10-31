package service

import (
	"campfire/internal/shared/model"
	"context"
	"errors"
	"time"
)

// FileDispatcher delivers file operations to an authenticated Spark agent.
type FileDispatcher interface {
	Files(context.Context, string, model.FileRequest) (model.FileResult, error)
}

// UploadLimit returns the configured per-file ceiling, defaulting to 1 GiB.
func (s *DeploymentService) UploadLimit() int64 {
	if s.MaxUploadBytes > 0 {
		return min(s.MaxUploadBytes, model.FileTransferLimit)
	}

	return model.DefaultUploadLimit
}

// Files targets the saved placement and coordinates operations with edits and deletion.
func (s *DeploymentService) Files(ctx context.Context, id string, request model.FileRequest) (model.FileResult, error) {
	// Only panel configuration can set the limit sent to Spark, never API input.
	request.UploadLimit = s.UploadLimit()
	if request.Operation == model.FileOperationUploadBegin && (request.Size < -1 || request.Size > request.UploadLimit) || request.Operation == model.FileOperationUploadChunk && (request.Offset < 0 || request.Offset > request.UploadLimit-int64(len(request.Data))) {
		return model.FileResult{}, &model.FileError{Code: "file_limit", Message: "Upload exceeds the configured maximum file size."}
	}

	timeout := 10 * time.Second
	if request.Operation == model.FileOperationCompress || request.Operation == model.FileOperationExtract {
		timeout = 30 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	release, err := s.lockServer(ctx, id)
	if err != nil {
		return model.FileResult{}, err
	}
	defer release()

	return s.filesUnlocked(ctx, id, request)
}

func (s *DeploymentService) filesUnlocked(ctx context.Context, id string, request model.FileRequest) (model.FileResult, error) {
	if s.Transfers != nil && request.Operation != model.FileOperationList && request.Operation != model.FileOperationRead && request.Operation != model.FileOperationStat && request.Operation != model.FileOperationReadChunk && request.Operation != model.FileOperationUploadCancel && request.Operation != model.FileOperationBackupList && request.Operation != model.FileOperationBackupRead {
		if err := s.Transfers.Store.Guard(ctx, id); err != nil {
			return model.FileResult{}, err
		}
	}

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
