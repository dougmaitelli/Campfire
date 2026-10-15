package model

// Deployment is the complete desired configuration delivered to a Spark.
type Deployment struct {
	ServerID      string              `json:"server_id"`
	Configuration ServerConfiguration `json:"configuration"`
	Ember         Ember               `json:"ember"`
}

// ConfigurationReceipt acknowledges durable configuration storage, not installation.
type ConfigurationReceipt struct {
	ServerID string `json:"server_id"`
	Revision string `json:"revision"`
}
