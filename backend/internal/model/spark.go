// Package model defines game server nodes and shared domain types.
package model

import "time"

// SparkKind identifies whether a node runs inside Campfire or in a separate agent.
type SparkKind string

// Supported Spark deployment types.
const (
	SparkLocal  SparkKind = "local"
	SparkRemote SparkKind = "remote"
)

// Spark is a registered game server node. Registration does not imply connectivity.
type Spark struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      SparkKind `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}
