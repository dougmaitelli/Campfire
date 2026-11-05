package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

// Job applies an archive operation using the server's stored deployment.
func (s *ConfigurationStore) Job(ctx context.Context, id string, request model.JobRequest) (model.JobResult, error) {
	if !utils.IsValidResourceID(id) || !utils.IsValidResourceID(request.ServerID) {
		return model.JobResult{}, errors.New("invalid transfer placement")
	}

	data, err := os.ReadFile(filepath.Join(s.Directory, id, request.ServerID+".json"))
	if errors.Is(err, os.ErrNotExist) && request.Operation == model.JobOperationDiscard {
		return model.JobResult{ID: request.ID, ServerID: request.ServerID, State: model.JobStateAbsent}, nil
	}

	if errors.Is(err, os.ErrNotExist) && request.Operation == model.JobOperationRelease {
		return model.JobResult{ID: request.ID, ServerID: request.ServerID, State: model.JobStateReleased}, nil
	}

	if err != nil {
		return model.JobResult{}, errors.New("deliver server configuration before transferring")
	}

	deployment, err := model.DecodeStoredDeployment(data)
	if err != nil {
		return model.JobResult{}, err
	}

	if deployment.ServerID != request.ServerID || deployment.Configuration.SparkID != id {
		return model.JobResult{}, errors.New("transfer placement mismatch")
	}

	runtime, ok := s.Runtime.(interface {
		Job(context.Context, model.Deployment, model.JobRequest) (model.JobResult, error)
	})
	if !ok {
		return model.JobResult{}, errors.New("spark transfer runtime unavailable")
	}

	return runtime.Job(ctx, deployment, request)
}
