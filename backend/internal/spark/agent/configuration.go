package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"time"

	"campfire/internal/shared/agentidentity"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

var configurationID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ConfigurationStore stores desired state on standalone Spark agents.
type ConfigurationStore struct {
	Update             func(context.Context, string) (string, error)
	RestartExecutable  string
	NegotiatedProtocol int
	Directory          string
	mu                 sync.Mutex
	Runtime            interface {
		Execute(context.Context, model.Deployment, model.RuntimeRequest) (model.RuntimeStatus, error)
		Files(context.Context, model.Deployment, model.FileRequest) (model.FileResult, error)
	}
}

func (s *ConfigurationStore) capabilities() []string {
	offered := protocol.SupportedCapabilities()
	if host, ok := s.Runtime.(interface{ DiskQuotaSupported() bool }); !ok || !host.DiskQuotaSupported() {
		offered = slices.DeleteFunc(offered, func(capability string) bool { return capability == protocol.CapabilityDiskQuota })
	}

	if s.Update == nil {
		offered = slices.DeleteFunc(offered, func(capability string) bool { return capability == protocol.CapabilitySelfUpdate })
	}

	return offered
}

// Files resolves the saved game definition and invokes the shared file manager.
func (s *ConfigurationStore) Files(ctx context.Context, id string, request model.FileRequest) (model.FileResult, error) {
	if !configurationID.MatchString(id) || !configurationID.MatchString(request.ServerID) {
		return model.FileResult{}, &model.FileError{Code: "invalid_file", Message: "Invalid server identity."}
	}

	data, err := os.ReadFile(filepath.Join(s.Directory, id, request.ServerID+".json"))
	if err != nil {
		return model.FileResult{}, &model.FileError{Code: "file_not_found", Message: "Install the server before managing its files."}
	}

	var deployment model.Deployment
	if err := json.Unmarshal(data, &deployment); err != nil {
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
	if !configurationID.MatchString(id) || !configurationID.MatchString(request.ServerID) {
		return model.RuntimeStatus{}, errors.New("invalid server identity")
	}

	data, err := os.ReadFile(filepath.Join(s.Directory, id, request.ServerID+".json"))
	if errors.Is(err, os.ErrNotExist) && request.Action == "status" {
		return model.RuntimeStatus{ServerID: request.ServerID, State: "not_installed"}, nil
	}

	if errors.Is(err, os.ErrNotExist) && request.Action == "metrics" {
		return model.RuntimeStatus{ServerID: request.ServerID, State: "not_installed", Metrics: &model.ResourceUsage{ServerID: request.ServerID, State: "not_installed", ContainerError: "Deliver configuration before collecting usage.", DiskError: "Deliver configuration before collecting usage."}}, nil
	}

	if err != nil {
		return model.RuntimeStatus{}, errors.New("deliver server configuration first")
	}

	var deployment model.Deployment
	if err := json.Unmarshal(data, &deployment); err != nil {
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

// Deploy validates placement and atomically persists a complete server definition.
func (s *ConfigurationStore) Deploy(ctx context.Context, id string, deployment model.Deployment) (model.ConfigurationReceipt, error) {
	if err := ctx.Err(); err != nil {
		return model.ConfigurationReceipt{}, err
	}

	if s.Directory == "" || !configurationID.MatchString(deployment.ServerID) || !configurationID.MatchString(id) || deployment.Configuration.SparkID != id || deployment.Configuration.EmberID != deployment.Ember.ID {
		return model.ConfigurationReceipt{}, errors.New("invalid configuration identity or placement")
	}

	if err := deployment.Ember.Validate(); err != nil {
		return model.ConfigurationReceipt{}, err
	}

	c := deployment.Configuration
	allowed := false

	for _, image := range deployment.Ember.Images {
		if c.Image == image {
			allowed = true
		}
	}

	if !allowed || c.Name == "" || math.IsNaN(c.CPULimit) || math.IsInf(c.CPULimit, 0) || c.CPULimit < 0.1 || c.CPULimit > 1024 || c.MemoryMB < 64 || c.MemoryMB > 1048576 || c.DiskMB < 0 || c.DiskMB > 1073741824 || len(c.Ports) == 0 || len(c.Ports) > 32 {
		return model.ConfigurationReceipt{}, errors.New("invalid server image or resources")
	}

	for _, port := range c.Ports {
		ip, err := netip.ParseAddr(port.BindIP)
		if err != nil || ip.Zone() != "" || ip.IsMulticast() || port.HostPort < 1 || port.HostPort > 65535 || port.ContainerPort < 1 || port.ContainerPort > 65535 || (port.Protocol != "tcp" && port.Protocol != "udp") {
			return model.ConfigurationReceipt{}, errors.New("invalid server port")
		}
	}

	for _, variable := range deployment.Ember.Variables {
		value := c.Variables[variable.Name]
		if variable.ValidateValue(value) != nil {
			return model.ConfigurationReceipt{}, errors.New("invalid server variable")
		}
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

	return model.ConfigurationReceipt{ServerID: deployment.ServerID, Revision: agentidentity.Fingerprint(data)}, nil
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
		result.Error = &protocol.CommandError{Code: "invalid_deadline", Message: "command deadline is invalid or expired"}
		return result
	}

	if command.Name == protocol.UpdateSpark {
		if s.Update == nil {
			result.Error = &protocol.CommandError{Code: "unsupported_capability", Message: "Restart Spark without an enrollment token before using remote updates."}
			return result
		}

		ctx, cancel := context.WithDeadline(ctx, command.Deadline)
		defer cancel()

		filename, err := s.Update(ctx, command.SparkVersion)
		if err != nil {
			result.Error = &protocol.CommandError{Code: "spark_update_failed", Message: fmt.Sprintf("%.1024s", err.Error())}
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
			result.Error = &protocol.CommandError{Code: "invalid_transfer", Message: "Transfer request required."}
			return result
		}

		transfer, err := s.Transfer(ctx, id, *command.Transfer)
		if err != nil {
			result.Error = &protocol.CommandError{Code: "transfer_failed", Message: fmt.Sprintf("%.1024s", err.Error())}
		} else {
			result.Transfer = &transfer
		}

		return result
	}

	if command.Name == protocol.ServerFiles {
		if command.Files == nil {
			result.Error = &protocol.CommandError{Code: "invalid_file", Message: "File request required."}
			return result
		}

		ctx, cancel := context.WithDeadline(ctx, command.Deadline)
		defer cancel()

		files, err := s.Files(ctx, id, *command.Files)
		if err != nil {
			var failure *model.FileError
			if errors.As(err, &failure) {
				result.Error = &protocol.CommandError{Code: failure.Code, Message: failure.Message}
			} else {
				result.Error = &protocol.CommandError{Code: "file_io", Message: "File operation unavailable."}
			}

			return result
		}

		result.Files = &files

		return result
	}

	if command.Name == protocol.ServerAction {
		if command.Runtime == nil {
			result.Error = &protocol.CommandError{Code: "invalid_operation", Message: "runtime request is required"}
			return result
		}

		ctx, cancel := context.WithDeadline(ctx, command.Deadline)
		defer cancel()

		ctx, timeout := context.WithTimeout(ctx, 5*time.Second)
		defer timeout()

		status, err := s.Action(ctx, id, *command.Runtime)
		if err != nil {
			result.Error = &protocol.CommandError{Code: "runtime_rejected", Message: err.Error()}
			return result
		}

		result.Status = &status

		return result
	}

	if command.Name != protocol.ConfigureServer {
		result.Error = &protocol.CommandError{Code: "unsupported_command", Message: "unsupported agent operation"}
		return result
	}

	if command.Deployment == nil {
		result.Error = &protocol.CommandError{Code: "invalid_configuration", Message: "configuration is required"}
		return result
	}

	ctx, cancel := context.WithDeadline(ctx, command.Deadline)
	defer cancel()

	receipt, err := s.Deploy(ctx, id, *command.Deployment)
	if err != nil {
		// Configuration may contain secrets; never return storage paths or raw payloads.
		result.Error = &protocol.CommandError{Code: "configuration_rejected", Message: "could not validate or persist configuration"}
		return result
	}

	result.Receipt = &receipt

	return result
}
