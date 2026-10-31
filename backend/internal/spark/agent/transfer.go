package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

// Transfer resolves a saved deployment before touching runtime storage.
func (s *ConfigurationStore) Transfer(ctx context.Context, id string, request model.TransferRequest) (model.TransferResult, error) {
	if !utils.IsValidResourceID(id) || !utils.IsValidResourceID(request.ServerID) {
		return model.TransferResult{}, errors.New("invalid transfer placement")
	}

	data, err := os.ReadFile(filepath.Join(s.Directory, id, request.ServerID+".json"))
	if errors.Is(err, os.ErrNotExist) && request.Operation == model.TransferOperationDiscard {
		return model.TransferResult{ID: request.ID, ServerID: request.ServerID, State: model.TransferStateAbsent}, nil
	}

	if errors.Is(err, os.ErrNotExist) && request.Operation == model.TransferOperationRelease {
		return model.TransferResult{ID: request.ID, ServerID: request.ServerID, State: model.TransferStateReleased}, nil
	}

	if err != nil {
		return model.TransferResult{}, errors.New("deliver server configuration before transferring")
	}

	deployment, err := model.DecodeStoredDeployment(data)
	if err != nil {
		return model.TransferResult{}, err
	}

	if deployment.ServerID != request.ServerID || deployment.Configuration.SparkID != id {
		return model.TransferResult{}, errors.New("transfer placement mismatch")
	}

	runtime, ok := s.Runtime.(interface {
		Transfer(context.Context, model.Deployment, model.TransferRequest) (model.TransferResult, error)
	})
	if !ok {
		return model.TransferResult{}, errors.New("spark transfer runtime unavailable")
	}

	return runtime.Transfer(ctx, deployment, request)
}
