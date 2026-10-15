package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"campfire/internal/shared/model"
)

// Transfer resolves a saved deployment before touching runtime storage.
func (s *ConfigurationStore) Transfer(ctx context.Context, id string, request model.TransferRequest) (model.TransferResult, error) {
	if !configurationID.MatchString(id) || !configurationID.MatchString(request.ServerID) {
		return model.TransferResult{}, errors.New("invalid transfer placement")
	}

	data, err := os.ReadFile(filepath.Join(s.Directory, id, request.ServerID+".json"))
	if errors.Is(err, os.ErrNotExist) && request.Operation == "discard" {
		return model.TransferResult{ID: request.ID, ServerID: request.ServerID, State: "absent"}, nil
	}

	if errors.Is(err, os.ErrNotExist) && request.Operation == "release" {
		return model.TransferResult{ID: request.ID, ServerID: request.ServerID, State: "released"}, nil
	}

	if err != nil {
		return model.TransferResult{}, errors.New("deliver server configuration before transferring")
	}

	var deployment model.Deployment
	if err := json.Unmarshal(data, &deployment); err != nil {
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
