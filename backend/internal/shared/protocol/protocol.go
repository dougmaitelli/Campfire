// Package protocol defines the versioned messages exchanged by Campfire and Spark.
package protocol

import (
	"time"

	"campfire/internal/shared/model"
)

// ProtocolVersion is the newest supported panel-agent wire format, independent
// of optional features advertised by each peer.
const ProtocolVersion = 1

// MinimumProtocolVersion bounds wire compatibility, not feature availability.
const MinimumProtocolVersion = 1

// CommandName identifies an explicitly supported agent operation.
type CommandName string

// ConfigureServer stores desired server configuration without executing it.
const ConfigureServer CommandName = "server.configure"

// ServerAction requests a lifecycle operation or status inspection.
const ServerAction CommandName = "server.action"

// ServerFiles invokes contained game data file management.
const ServerFiles CommandName = "server.files"

// ServerTransfer moves one fenced archive through the authenticated session.
const ServerTransfer CommandName = "server.transfer"

// UpdateSpark replaces the agent executable with the verified panel build.
const UpdateSpark CommandName = "spark.update"

// PruneSpark removes unused Campfire Docker resources on a node.
const PruneSpark CommandName = "spark.prune"

// SparkUpdateInfo lets the running agent verify a staged binary's compatibility.
type SparkUpdateInfo struct {
	Version         string `json:"version"`
	MinimumProtocol int    `json:"minimum_protocol"`
	Protocol        int    `json:"protocol"`
}

// Command is a correlated, bounded-lifetime request.
type Command struct {
	ID           string                 `json:"id"`
	Name         CommandName            `json:"name"`
	Deadline     time.Time              `json:"deadline"`
	Deployment   *model.Deployment      `json:"deployment,omitempty"`
	Runtime      *model.RuntimeRequest  `json:"runtime,omitempty"`
	Files        *model.FileRequest     `json:"files,omitempty"`
	Transfer     *model.TransferRequest `json:"transfer,omitempty"`
	SparkVersion string                 `json:"spark_version,omitempty"`
}

// CommandError describes a failed operation independently of transport failure.
type CommandError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *CommandError) Error() string { return e.Code + ": " + e.Message }

// CommandResponse completes exactly one request on the originating session.
type CommandResponse struct {
	Prune        *model.PruneResult          `json:"prune,omitempty"`
	ID           string                      `json:"id"`
	Receipt      *model.ConfigurationReceipt `json:"receipt,omitempty"`
	Error        *CommandError               `json:"error,omitempty"`
	Status       *model.RuntimeStatus        `json:"status,omitempty"`
	Files        *model.FileResult           `json:"files,omitempty"`
	Transfer     *model.TransferResult       `json:"transfer,omitempty"`
	SparkVersion string                      `json:"spark_version,omitempty"`
}

// Enrollment returns a signed identity after consuming a bootstrap token.
type Enrollment struct {
	ID          string `json:"id"`
	Certificate []byte `json:"certificate"`
	CA          []byte `json:"ca"`
}

// Message carries versioned agent handshake and heartbeat data.
type Message struct {
	MinimumProtocol int              `json:"minimum_protocol,omitempty"`
	Capabilities    []string         `json:"capabilities,omitempty"`
	Type            string           `json:"type"`
	Protocol        int              `json:"protocol"`
	SparkID         string           `json:"spark_id,omitempty"`
	Hostname        string           `json:"hostname,omitempty"`
	Version         string           `json:"version,omitempty"`
	Command         *Command         `json:"command,omitempty"`
	Response        *CommandResponse `json:"response,omitempty"`
	Console         *ConsoleFrame    `json:"console,omitempty"`
	Game            *GameFrame       `json:"game,omitempty"`
	GameAck         *GameAck         `json:"game_ack,omitempty"`
}

// GameFrame binds a local publication to Spark's authenticated server identity.
type GameFrame struct {
	ID          string                `json:"id"`
	ServerID    string                `json:"server_id"`
	Publication model.GamePublication `json:"publication"`
}

// GameAck confirms ingestion. With CapabilityGameEvents an event acknowledgement
// follows the database commit; external delivery is a separate concern.
type GameAck struct {
	ID    string        `json:"id"`
	Error *CommandError `json:"error,omitempty"`
}

// ConsoleFrame identifies one bounded console stream message on its originating session.
type ConsoleFrame struct {
	ID       string             `json:"id"`
	ServerID string             `json:"server_id"`
	Event    model.ConsoleEvent `json:"event"`
}
