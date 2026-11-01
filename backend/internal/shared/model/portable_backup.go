package model

import "maps"

// BackupDefinition preserves authoring inputs for recreation, including hidden
// variable values. It belongs only to backup archives, never runtime configuration.
type BackupDefinition struct {
	Ember         Ember               `json:"ember"`
	Configuration ServerConfiguration `json:"configuration"`
}

// NewBackupDefinition excludes source placement and port bindings. Destination
// ownership and access grants are deliberately absent from the portable model.
func NewBackupDefinition(server Server, ember Ember) *BackupDefinition {
	configuration := server.ServerConfiguration
	configuration.SparkID = ""
	configuration.Ports = nil
	configuration.Variables = maps.Clone(configuration.Variables)

	return &BackupDefinition{Ember: ember, Configuration: configuration}
}
