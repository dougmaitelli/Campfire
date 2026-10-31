package model

import (
	"encoding/json"
	"errors"
)

// decodeLegacyDeployment converts full-definition snapshots into execution data.
//
// Deprecated: Remove after the next release, once all nodes have been migrated.
func decodeLegacyDeployment(data []byte) (Deployment, error) {
	var legacy struct {
		ServerID      string              `json:"server_id"`
		Configuration ServerConfiguration `json:"configuration"`
		Ember         Ember               `json:"ember"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return Deployment{}, err
	}

	if legacy.Configuration.EmberID != legacy.Ember.ID {
		return Deployment{}, errors.New("stored Ember identity mismatch")
	}

	return NewDeployment(legacy.ServerID, legacy.Configuration, legacy.Ember)
}
