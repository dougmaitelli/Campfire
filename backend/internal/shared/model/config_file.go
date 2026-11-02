package model

import (
	"bytes"
	"encoding/json"
)

// UnmarshalJSON accepts the retired strings option in existing definitions while
// omitting it from the model and all newly serialized Embers and deployments.
func (c *ConfigFile) UnmarshalJSON(data []byte) error {
	type configFile ConfigFile

	var definition struct {
		configFile
		// TODO: Remove once older definitions containing "strings" no longer need support.
		LegacyStrings json.RawMessage `json:"strings"`
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&definition); err != nil {
		return err
	}

	*c = ConfigFile(definition.configFile)

	return nil
}
