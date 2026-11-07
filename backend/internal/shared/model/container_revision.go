package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ContainerRevision identifies settings applied when creating the game container.
// Installer, stop/readiness rules, log monitoring, and panel backup paths are
// deliberately excluded: updating them does not require recreating the container.
func ContainerRevision(deployment Deployment) string {
	if deployment.Ember.StartupShell == "" {
		deployment.Ember.StartupShell = "sh"
	}

	if len(deployment.Configuration.Variables) == 0 {
		deployment.Configuration.Variables = nil
	}

	if len(deployment.Configuration.Ports) == 0 {
		deployment.Configuration.Ports = nil
	}

	if len(deployment.Ember.Configs) == 0 {
		deployment.Ember.Configs = nil
	}

	data, _ := json.Marshal(struct {
		Configuration RuntimeConfiguration
		Startup       string
		Shell         string
		WorkDir       string
		User          string
		Configs       []ConfigFile
	}{deployment.Configuration, deployment.Ember.Startup, deployment.Ember.StartupShell, deployment.Ember.WorkDir, deployment.Ember.User, deployment.Ember.Configs})
	hash := sha256.Sum256(data)

	return hex.EncodeToString(hash[:])
}
