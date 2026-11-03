package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"campfire/internal/shared/agentidentity"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
	"campfire/internal/shared/utils"
)

// ConfigurationStore stores desired state on standalone Spark agents.
type ConfigurationStore struct {
	Update             func(context.Context, string, bool) (string, error)
	RestartExecutable  string
	NegotiatedProtocol int
	Directory          string
	mu                 sync.Mutex
	games              *gameIntegrations
	Runtime            interface {
		Execute(context.Context, model.Deployment, model.RuntimeRequest) (model.RuntimeStatus, error)
		Files(context.Context, model.Deployment, model.FileRequest) (model.FileResult, error)
	}
}

func (s *ConfigurationStore) capabilities() []string {
	offered := protocol.SupportedCapabilities()
	if runtime, ok := s.Runtime.(interface{ SupportsLogMonitoring() bool }); !ok || !runtime.SupportsLogMonitoring() {
		offered = slices.DeleteFunc(offered, func(capability string) bool { return capability == protocol.CapabilityLogAlerts })
	}

	if s.games == nil {
		offered = slices.DeleteFunc(offered, func(capability string) bool {
			return capability == protocol.CapabilityGameIntegration || capability == protocol.CapabilityGameEvents || capability == protocol.CapabilityGameMedia
		})
	}

	if host, ok := s.Runtime.(interface{ DiskQuotaSupported() bool }); !ok || !host.DiskQuotaSupported() {
		offered = slices.DeleteFunc(offered, func(capability string) bool { return capability == protocol.CapabilityDiskQuota })
	}

	if s.Update == nil {
		offered = slices.DeleteFunc(offered, func(capability string) bool {
			return capability == protocol.CapabilitySelfUpdate || capability == protocol.CapabilitySelfReinstall
		})
	}

	if _, ok := s.Runtime.(interface {
		Prune(context.Context) (model.PruneResult, error)
	}); !ok {
		offered = slices.DeleteFunc(offered, func(capability string) bool { return capability == protocol.CapabilityPrune })
	}

	return offered
}

// Files resolves the saved game definition and invokes the shared file manager.
func (s *ConfigurationStore) Files(ctx context.Context, id string, request model.FileRequest) (model.FileResult, error) {
	if !utils.IsValidResourceID(id) || !utils.IsValidResourceID(request.ServerID) {
		return model.FileResult{}, &model.FileError{Code: model.FileInvalid, Message: "Invalid server identity."}
	}

	data, err := os.ReadFile(filepath.Join(s.Directory, id, request.ServerID+".json"))
	if err != nil {
		return model.FileResult{}, &model.FileError{Code: model.FileNotFound, Message: "Install the server before managing its files."}
	}

	deployment, err := model.DecodeStoredDeployment(data)
	if err != nil {
		return model.FileResult{}, errors.New("invalid stored configuration")
	}

	if deployment.ServerID != request.ServerID || deployment.Configuration.SparkID != id {
		return model.FileResult{}, errors.New("configuration placement mismatch")
	}

	if s.Runtime == nil {
		return model.FileResult{}, errors.New("file runtime unavailable")
	}

	return s.Runtime.Files(ctx, deployment, request)
}

// Action loads the locally saved definition and invokes the shared runtime.
func (s *ConfigurationStore) Action(ctx context.Context, id string, request model.RuntimeRequest) (model.RuntimeStatus, error) {
	if !utils.IsValidResourceID(id) || !utils.IsValidResourceID(request.ServerID) {
		return model.RuntimeStatus{}, errors.New("invalid server identity")
	}

	data, err := os.ReadFile(filepath.Join(s.Directory, id, request.ServerID+".json"))
	if errors.Is(err, os.ErrNotExist) && request.Action == model.ServerActionStatus {
		return model.RuntimeStatus{ServerID: request.ServerID, State: model.RuntimeStateNotInstalled}, nil
	}

	if errors.Is(err, os.ErrNotExist) && request.Action == model.ServerActionMetrics {
		return model.RuntimeStatus{ServerID: request.ServerID, State: model.RuntimeStateNotInstalled, Metrics: &model.ResourceUsage{ServerID: request.ServerID, State: model.RuntimeStateNotInstalled, ContainerError: "Deliver configuration before collecting usage.", DiskError: "Deliver configuration before collecting usage."}}, nil
	}

	if err != nil {
		return model.RuntimeStatus{}, errors.New("deliver server configuration first")
	}

	deployment, err := model.DecodeStoredDeployment(data)
	if err != nil {
		return model.RuntimeStatus{}, err
	}

	if deployment.ServerID != request.ServerID || deployment.Configuration.SparkID != id {
		return model.RuntimeStatus{}, errors.New("configuration placement mismatch")
	}

	if s.Runtime == nil {
		return model.RuntimeStatus{}, errors.New("docker runtime is not enabled")
	}

	return s.Runtime.Execute(ctx, deployment, request)
}

// Deploy validates placement and atomically persists execution instructions.
func (s *ConfigurationStore) Deploy(ctx context.Context, id string, deployment model.Deployment) (model.ConfigurationReceipt, error) {
	if err := ctx.Err(); err != nil {
		return model.ConfigurationReceipt{}, err
	}

	if s.Directory == "" || !utils.IsValidResourceID(deployment.ServerID) || !utils.IsValidResourceID(id) || deployment.Configuration.SparkID != id {
		return model.ConfigurationReceipt{}, errors.New("invalid configuration identity or placement")
	}

	if err := deployment.Validate(); err != nil {
		return model.ConfigurationReceipt{}, err
	}

	data, err := json.Marshal(deployment)
	if err != nil {
		return model.ConfigurationReceipt{}, err
	}

	if len(data) > 1<<20 {
		return model.ConfigurationReceipt{}, errors.New("configuration exceeds storage limits")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return model.ConfigurationReceipt{}, err
	}

	if err := agentidentity.WritePrivate(filepath.Join(s.Directory, id, deployment.ServerID+".json"), data); err != nil {
		return model.ConfigurationReceipt{}, err
	}

	if s.games != nil && !deployment.Configuration.GameIntegration {
		s.games.unmount(deployment.ServerID)
	}

	if monitor, ok := s.Runtime.(interface{ WatchLogs(model.Deployment) error }); ok {
		if err := monitor.WatchLogs(deployment); err != nil {
			return model.ConfigurationReceipt{}, err
		}
	}

	return model.ConfigurationReceipt{ServerID: deployment.ServerID, Revision: agentidentity.Fingerprint(data)}, nil
}

// RestoreLogMonitors reattaches monitors to containers surviving a Spark restart.
func (s *ConfigurationStore) RestoreLogMonitors(id string) error {
	monitor, ok := s.Runtime.(interface{ WatchLogs(model.Deployment) error })
	if !ok {
		return nil
	}

	entries, err := os.ReadDir(filepath.Join(s.Directory, id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.Directory, id, entry.Name()))
		if err != nil {
			return err
		}

		deployment, err := model.DecodeStoredDeployment(data)
		if err != nil || deployment.Configuration.SparkID != id || deployment.ServerID+".json" != entry.Name() {
			return errors.New("invalid saved monitoring placement")
		}

		if err := monitor.WatchLogs(deployment); err != nil {
			return err
		}
	}

	return nil
}

func (s *ConfigurationStore) executeNegotiated(ctx context.Context, id string, command protocol.Command, capabilities []string) protocol.CommandResponse {
	if err := protocol.Require(capabilities, protocol.RequiredCapabilities(command)...); err != nil {
		var failure *protocol.CommandError
		if errors.As(err, &failure) {
			return protocol.CommandResponse{ID: command.ID, Error: failure}
		}
	}

	return s.execute(ctx, id, command)
}

func (s *ConfigurationStore) execute(ctx context.Context, id string, command protocol.Command) protocol.CommandResponse {
	result := protocol.CommandResponse{ID: command.ID}
	if command.ID == "" || len(command.ID) > 128 || command.Deadline.IsZero() || !command.Deadline.After(time.Now()) || command.Deadline.After(time.Now().Add(protocol.CommandTimeout(command)+time.Second)) {
		result.Error = &protocol.CommandError{Code: protocol.ErrorInvalidDeadline, Message: "command deadline is invalid or expired"}
		return result
	}

	if command.Name == protocol.PruneSpark {
		pruner, ok := s.Runtime.(interface {
			Prune(context.Context) (model.PruneResult, error)
		})
		if !ok {
			result.Error = &protocol.CommandError{Code: protocol.ErrorUnsupportedCapability, Message: "Docker cleanup is unavailable on this node."}
			return result
		}

		ctx, cancel := context.WithDeadline(ctx, command.Deadline)
		defer cancel()

		pruned, err := pruner.Prune(ctx)
		if err != nil {
			result.Error = &protocol.CommandError{Code: protocol.ErrorPruneFailed, Message: err.Error()}
		} else {
			result.Prune = &pruned
		}

		return result
	}

	if command.Name == protocol.UpdateSpark || command.Name == protocol.ReinstallSpark {
		if s.Update == nil {
			result.Error = &protocol.CommandError{Code: protocol.ErrorUnsupportedCapability, Message: "Restart Spark without an enrollment token before using remote updates."}
			return result
		}

		ctx, cancel := context.WithDeadline(ctx, command.Deadline)
		defer cancel()

		filename, err := s.Update(ctx, command.SparkVersion, command.Name == protocol.ReinstallSpark)
		if err != nil {
			result.Error = &protocol.CommandError{Code: protocol.ErrorUpdateFailed, Message: fmt.Sprintf("%.1024s", err.Error())}
			return result
		}

		s.RestartExecutable = filename
		result.SparkVersion = command.SparkVersion

		return result
	}

	if command.Name == protocol.ServerTransfer {
		ctx, cancel := context.WithDeadline(ctx, command.Deadline)
		defer cancel()

		if command.Transfer == nil {
			result.Error = &protocol.CommandError{Code: protocol.ErrorInvalidTransfer, Message: "Transfer request required."}
			return result
		}

		transfer, err := s.Transfer(ctx, id, *command.Transfer)
		if err != nil {
			result.Error = &protocol.CommandError{Code: protocol.ErrorTransferFailed, Message: fmt.Sprintf("%.1024s", err.Error())}
		} else {
			result.Transfer = &transfer
		}

		return result
	}

	if command.Name == protocol.ServerFiles {
		if command.Files == nil {
			result.Error = &protocol.CommandError{Code: protocol.CommandErrorCode(model.FileInvalid), Message: "File request required."}
			return result
		}

		ctx, cancel := context.WithDeadline(ctx, command.Deadline)
		defer cancel()

		files, err := s.Files(ctx, id, *command.Files)
		if err != nil {
			var failure *model.FileError
			if errors.As(err, &failure) {
				result.Error = &protocol.CommandError{Code: protocol.CommandErrorCode(failure.Code), Message: failure.Message}
			} else {
				result.Error = &protocol.CommandError{Code: protocol.CommandErrorCode(model.FileIO), Message: "File operation unavailable."}
			}

			return result
		}

		result.Files = &files

		return result
	}

	if command.Name == protocol.ServerAction {
		if command.Runtime == nil {
			result.Error = &protocol.CommandError{Code: protocol.ErrorInvalidOperation, Message: "runtime request is required"}
			return result
		}

		ctx, cancel := context.WithDeadline(ctx, command.Deadline)
		defer cancel()

		ctx, timeout := context.WithTimeout(ctx, 5*time.Second)
		defer timeout()

		status, err := s.Action(ctx, id, *command.Runtime)
		if err != nil {
			result.Error = &protocol.CommandError{Code: protocol.ErrorRuntimeRejected, Message: err.Error()}
			return result
		}

		result.Status = &status

		return result
	}

	if command.Name != protocol.ConfigureServer {
		result.Error = &protocol.CommandError{Code: protocol.ErrorUnsupportedCommand, Message: "unsupported agent operation"}
		return result
	}

	if command.Deployment == nil {
		result.Error = &protocol.CommandError{Code: protocol.ErrorInvalidConfiguration, Message: "configuration is required"}
		return result
	}

	ctx, cancel := context.WithDeadline(ctx, command.Deadline)
	defer cancel()

	receipt, err := s.Deploy(ctx, id, *command.Deployment)
	if err != nil {
		// Configuration may contain secrets; never return storage paths or raw payloads.
		result.Error = &protocol.CommandError{Code: protocol.ErrorConfigurationRejected, Message: "could not validate or persist configuration"}
		return result
	}

	result.Receipt = &receipt

	return result
}
