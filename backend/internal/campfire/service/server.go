package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/netip"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// Server errors describe invalid configurations and reservation conflicts.
var (
	ErrInvalidServer  = errors.New("invalid server configuration")
	ErrServerNotFound = errors.New("server not found")
	ErrServerConflict = errors.New("server name, resource capacity, or port reservation conflicts")
	ErrServerActive   = errors.New("stop the server before changing its configuration")
)

// ServerStore persists server configurations and their reservations.
type ServerStore interface {
	Save(context.Context, model.Server, bool) error
	List(context.Context) ([]model.Server, error)
	Get(context.Context, string) (model.Server, error)
	Delete(context.Context, string) error
}

// ServerService validates configurations against current Ember definitions.
type ServerService struct {
	store   ServerStore
	embers  *EmberService
	runtime interface {
		BeforeUpdate(context.Context, string) (func(), error)
		BeforeEdit(context.Context, string, model.ServerConfiguration) (func(), error)
	}
}

// WithRuntimeGuard serializes edits with lifecycle and reservation changes.
func (s *ServerService) WithRuntimeGuard(guard interface {
	BeforeUpdate(context.Context, string) (func(), error)
	BeforeEdit(context.Context, string, model.ServerConfiguration) (func(), error)
}) *ServerService {
	s.runtime = guard
	return s
}

// NewServerService constructs the server application service.
func NewServerService(store ServerStore, embers *EmberService) *ServerService {
	return &ServerService{store: store, embers: embers}
}

// Create validates configuration and reserves resources on the selected Spark.
func (s *ServerService) Create(ctx context.Context, input model.ServerConfiguration) (model.Server, error) {
	return s.CreateOwned(ctx, input, 0)
}

// CreateOwned assigns ownership in the same transaction as capacity reservation.
func (s *ServerService) CreateOwned(ctx context.Context, input model.ServerConfiguration, ownerID int64) (model.Server, error) {
	server := model.Server{ID: rand.Text(), ServerConfiguration: input, CreatedAt: time.Now().UTC()}
	server.OwnerUserID = ownerID

	return s.save(ctx, server, true)
}

// Update changes a configuration while preserving placement and game identity.
func (s *ServerService) Update(ctx context.Context, id string, input model.ServerConfiguration) (model.Server, error) {
	if s.runtime != nil {
		release, err := s.runtime.BeforeEdit(ctx, id, input)
		if err != nil {
			return model.Server{}, err
		}
		defer release()
	}

	server, err := s.Get(ctx, id)
	if err != nil {
		return model.Server{}, err
	}

	if input.SparkID != server.SparkID || input.EmberID != server.EmberID {
		return model.Server{}, fmt.Errorf("%w: Spark and Ember cannot be changed", ErrInvalidServer)
	}

	input, err = s.preserveHiddenVariables(ctx, server, input)
	if err != nil {
		return model.Server{}, err
	}

	server.ServerConfiguration = input

	return s.save(ctx, server, false)
}

// preserveHiddenVariables hydrates omitted values from the locked server snapshot.
func (s *ServerService) preserveHiddenVariables(ctx context.Context, current model.Server, input model.ServerConfiguration) (model.ServerConfiguration, error) {
	ember, err := s.embers.Get(ctx, current.EmberID)
	if err != nil {
		return model.ServerConfiguration{}, err
	}

	input.Variables = maps.Clone(input.Variables)
	if input.Variables == nil {
		input.Variables = make(map[string]string)
	}

	visible := make(map[string]bool)
	for _, variable := range ember.Variables {
		visible[variable.Name] = !variable.Hidden
	}
	// Hidden values omitted by the editor must survive edits, including while
	// the current Ember differs from the server's applied definition.
	for name, value := range current.Variables {
		if _, supplied := input.Variables[name]; !supplied && !visible[name] {
			input.Variables[name] = value
		}
	}

	return input, nil
}

// ApplyEmber explicitly replaces a stopped server's pinned definition. The
// installer is never run implicitly and the Ember identity remains unchanged.
func (s *ServerService) ApplyEmber(ctx context.Context, id, expected string, input model.ServerConfiguration) (model.Server, error) {
	if s.runtime == nil {
		return model.Server{}, ErrDeploymentUnavailable
	}

	release, err := s.runtime.BeforeUpdate(ctx, id)
	if err != nil {
		return model.Server{}, err
	}
	defer release()

	server, err := s.Get(ctx, id)
	if err != nil {
		return model.Server{}, err
	}

	if server.EmberID != expected {
		return model.Server{}, fmt.Errorf("%w: the pinned Ember changed; reopen the revision editor", ErrServerConflict)
	}

	if input.SparkID != server.SparkID || input.EmberID != server.EmberID {
		return model.Server{}, fmt.Errorf("%w: preserve the server's Spark and Ember", ErrInvalidServer)
	}

	latest, err := s.embers.Get(ctx, server.EmberID)
	if err != nil {
		return model.Server{}, err
	}

	if latest.Revision == server.EmberHash {
		return model.Server{}, fmt.Errorf("%w: this server already uses the current Ember", ErrServerConflict)
	}

	server.EmberHash = latest.Revision

	input.Variables = maps.Clone(input.Variables)
	if input.Variables == nil {
		input.Variables = make(map[string]string)
	}

	for _, variable := range latest.Variables {
		if _, supplied := input.Variables[variable.Name]; supplied || !variable.Hidden || !variable.Editable {
			continue
		}

		if value, exists := server.Variables[variable.Name]; exists {
			input.Variables[variable.Name] = value
		}
	}

	server.ServerConfiguration = input

	return s.save(ctx, server, false)
}

func (s *ServerService) save(ctx context.Context, server model.Server, create bool) (model.Server, error) {
	if server.Ports == nil {
		server.Ports = []model.ServerPort{}
	}
	server.Name = strings.TrimSpace(server.Name)
	if server.Name == "" || !utf8.ValidString(server.Name) || utf8.RuneCountInString(server.Name) > 100 || strings.ContainsFunc(server.Name, unicode.IsControl) ||
		math.IsNaN(server.CPULimit) || math.IsInf(server.CPULimit, 0) || server.CPULimit < 0.1 || server.CPULimit > 1024 ||
		server.MemoryMB < 64 || server.MemoryMB > 1048576 || server.DiskMB < 0 || server.DiskMB > 1073741824 ||
		len(server.Ports) > 32 {
		return model.Server{}, fmt.Errorf("%w: provide a name, 0.1–1024 CPU cores, at least 64 MiB memory, positive disk capacity, and at most 32 ports", ErrInvalidServer)
	}

	description, err := validateDescription(server.Description)
	if err != nil {
		return model.Server{}, fmt.Errorf("%w: %w", ErrInvalidServer, err)
	}

	server.Description = description
	if !create && server.EmberUpdateAvailable {
		current, err := s.Get(ctx, server.ID)
		if err != nil {
			return model.Server{}, err
		}

		if server.EmberHash == current.EmberHash {
			if !model.SameRuntimeConfiguration(current.ServerConfiguration, server.ServerConfiguration) {
				return model.Server{}, fmt.Errorf("%w: apply the current Ember before changing game configuration", ErrInvalidServer)
			}

			if err := s.store.Save(ctx, server, false); err != nil {
				return model.Server{}, err
			}

			return s.Get(ctx, server.ID)
		}
	}

	ember, err := s.embers.Get(ctx, server.EmberID)
	if err != nil {
		return model.Server{}, err
	}

	if create {
		server.EmberHash = ember.Revision
	}

	allowed := false

	for _, image := range ember.Images {
		if image == server.Image {
			allowed = true
		}
	}

	if !allowed {
		return model.Server{}, fmt.Errorf("%w: image is not allowed by this Ember", ErrInvalidServer)
	}

	selection, _, err := ember.ResolveStartup(server.StartupCommand)
	if err != nil {
		return model.Server{}, fmt.Errorf("%w: %w", ErrInvalidServer, err)
	}

	server.StartupCommand = selection

	for i := range server.Ports {
		port := &server.Ports[i]

		ip, err := netip.ParseAddr(port.BindIP)
		if err != nil || ip.Zone() != "" || ip.IsMulticast() || port.HostPort < 1 || port.HostPort > 65535 || port.ContainerPort < 1 || port.ContainerPort > 65535 || (port.Protocol != "tcp" && port.Protocol != "udp") {
			return model.Server{}, fmt.Errorf("%w: invalid port mapping", ErrInvalidServer)
		}

		port.BindIP = ip.Unmap().String()
	}

	for _, required := range ember.RequiredPorts {
		if len(server.Ports) == 0 {
			break
		}
		found := false

		for _, port := range server.Ports {
			if port.ContainerPort == required.Container && port.Protocol == required.Protocol {
				found = true
			}
		}

		if !found {
			return model.Server{}, fmt.Errorf("%w: Ember requires container port %d/%s", ErrInvalidServer, required.Container, required.Protocol)
		}
	}

	if server.Variables == nil {
		server.Variables = make(map[string]string)
	}

	known := make(map[string]bool, len(ember.Variables))
	for _, variable := range ember.Variables {
		known[variable.Name] = true

		value, supplied := server.Variables[variable.Name]
		if !supplied {
			value = variable.Default
		}

		if variable.ValidateValue(value) != nil || (!variable.Editable && value != variable.Default) {
			return model.Server{}, fmt.Errorf("%w: invalid variable %s", ErrInvalidServer, variable.Name)
		}

		server.Variables[variable.Name] = value
	}

	for key := range server.Variables {
		if !known[key] {
			return model.Server{}, fmt.Errorf("%w: unknown variable %s", ErrInvalidServer, key)
		}
	}

	if err := s.store.Save(ctx, server, create); err != nil {
		return model.Server{}, serverError(err)
	}

	return s.Get(ctx, server.ID)
}

func serverError(err error) error {
	switch {
	case errors.Is(err, repository.ErrServerNotFound):
		return ErrServerNotFound
	case errors.Is(err, repository.ErrServerConflict):
		return ErrServerConflict
	case errors.Is(err, repository.ErrServerPlacement):
		return fmt.Errorf("%w: %w", ErrInvalidServer, err)
	default:
		return err
	}
}

// List returns saved server configurations.
func (s *ServerService) List(ctx context.Context) ([]model.Server, error) { return s.store.List(ctx) }

// Get returns a server configuration.
func (s *ServerService) Get(ctx context.Context, id string) (model.Server, error) {
	server, err := s.store.Get(ctx, id)
	return server, serverError(err)
}

// Delete releases a server configuration's reservations.
func (s *ServerService) Delete(ctx context.Context, id string) error {
	return serverError(s.store.Delete(ctx, id))
}
