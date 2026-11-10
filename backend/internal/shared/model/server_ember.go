package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
)

// DeploymentHash identifies execution inputs, excluding authoring and backup metadata.
func DeploymentHash(deployment Deployment) string {
	data, _ := json.Marshal(deployment)
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:])
}

// ProposedServerConfiguration retains editable choices and proposes every new,
// removed, or Ember-managed value explicitly for the server review workflow.
func (e Ember) ProposedServerConfiguration(current ServerConfiguration) ServerConfiguration {
	next := current
	next.Ports = slices.Clone(current.Ports)

	next.Variables = make(map[string]string, len(e.Variables))
	for _, variable := range e.Variables {
		value, exists := current.Variables[variable.Name]
		if !exists || !variable.Editable {
			value = variable.Default
		}

		next.Variables[variable.Name] = value
	}

	if selection, _, err := e.ResolveStartup(current.StartupCommand); err == nil {
		next.StartupCommand = selection
	} else {
		next.StartupCommand = ""

		if len(e.StartupCommands) == 1 {
			for name := range e.StartupCommands {
				next.StartupCommand = name
			}
		}
	}

	allowed := false
	for _, image := range e.Images {
		allowed = allowed || image == current.Image
	}

	if !allowed {
		labels := slices.Sorted(maps.Keys(e.Images))
		if len(labels) > 0 {
			next.Image = e.Images[labels[0]]
		}
	}

	return next
}

// ServerNeedsEmberReview covers compatible value additions as well as invalid
// existing choices. Synchronization must never silently adopt proposed values.
func (e Ember) ServerNeedsEmberReview(server Server) bool {
	_, reviewRequired := e.serverDeployment(server)

	return reviewRequired
}

// serverDeployment evaluates saved values against a persisted, validated Ember.
// It returns the resolved deployment so status flags need only one evaluation.
func (e Ember) serverDeployment(server Server) (Deployment, bool) {
	current := server.ServerConfiguration
	if !SameRuntimeConfiguration(current, e.ProposedServerConfiguration(current)) {
		return Deployment{}, true
	}

	if len(current.Ports) > 0 {
		for _, required := range e.RequiredPorts {
			if !slices.ContainsFunc(current.Ports, func(port ServerPort) bool {
				return port.ContainerPort == required.Container && port.Protocol == required.Protocol
			}) {
				return Deployment{}, true
			}
		}
	}

	deployment, err := resolveDeployment(server.ID, current, e)
	if err != nil {
		return Deployment{}, true
	}

	_, err = deployment.Configuration.validate()

	return deployment, err != nil
}

// SetEmberStatus computes review and delivery independently of full Ember hashes.
// EmberHash is the existing stored fingerprint of the last acknowledged deployment.
func (s *Server) SetEmberStatus(ember Ember) {
	deployment, reviewRequired := ember.serverDeployment(*s)
	s.EmberReviewRequired = reviewRequired

	s.DeploymentSyncRequired = false
	if !s.EmberReviewRequired {
		s.DeploymentSyncRequired = s.EmberHash != DeploymentHash(deployment)
	}

	s.EmberUpdateAvailable = s.EmberReviewRequired || s.DeploymentSyncRequired
	s.SavedataAvailable = len(ember.SavedataPaths) > 0
}
