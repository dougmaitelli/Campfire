package spark

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/netip"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"campfire/internal/model"
)

var configurationID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ConfigurationStore stores desired state for remote agents and in-process local nodes.
type ConfigurationStore struct {
	Directory string
	mu        sync.Mutex
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

	if !allowed || c.Name == "" || math.IsNaN(c.CPULimit) || math.IsInf(c.CPULimit, 0) || c.CPULimit < 0.1 || c.CPULimit > 1024 || c.MemoryMB < 64 || c.MemoryMB > 1048576 || c.DiskMB < 1 || c.DiskMB > 1073741824 || len(c.Ports) == 0 || len(c.Ports) > 32 {
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
		if variable.Required && value == "" {
			return model.ConfigurationReceipt{}, errors.New("missing required variable")
		}

		if variable.Pattern != "" {
			pattern, err := regexp.Compile(variable.Pattern)
			if err != nil || !pattern.MatchString(value) {
				return model.ConfigurationReceipt{}, errors.New("invalid server variable")
			}
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

	if err := savePrivate(filepath.Join(s.Directory, id, deployment.ServerID+".json"), data); err != nil {
		return model.ConfigurationReceipt{}, err
	}

	return model.ConfigurationReceipt{ServerID: deployment.ServerID, Revision: fingerprint(data)}, nil
}

func (s *ConfigurationStore) execute(ctx context.Context, id string, command Command) CommandResponse {
	result := CommandResponse{ID: command.ID}
	if command.ID == "" || len(command.ID) > 128 || command.Deadline.IsZero() || !command.Deadline.After(time.Now()) || command.Deadline.After(time.Now().Add(31*time.Second)) {
		result.Error = &CommandError{Code: "invalid_deadline", Message: "command deadline is invalid or expired"}
		return result
	}

	if command.Name != ConfigureServer {
		result.Error = &CommandError{Code: "unsupported_command", Message: "unsupported agent operation"}
		return result
	}

	if command.Deployment == nil {
		result.Error = &CommandError{Code: "invalid_configuration", Message: "configuration is required"}
		return result
	}

	ctx, cancel := context.WithDeadline(ctx, command.Deadline)
	defer cancel()

	receipt, err := s.Deploy(ctx, id, *command.Deployment)
	if err != nil {
		// Configuration may contain secrets; never return storage paths or raw payloads.
		result.Error = &CommandError{Code: "configuration_rejected", Message: "could not validate or persist configuration"}
		return result
	}

	result.Receipt = &receipt

	return result
}
