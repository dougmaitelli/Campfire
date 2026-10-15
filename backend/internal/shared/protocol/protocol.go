// Package protocol defines the versioned messages exchanged by Campfire and Spark.
package protocol

import (
	"time"

	"campfire/internal/shared/model"
)

// ProtocolVersion is the supported panel-agent wire protocol.
const ProtocolVersion = 6

// CommandName identifies an explicitly supported agent operation.
type CommandName string

// ConfigureServer stores desired server configuration without executing it.
const ConfigureServer CommandName = "server.configure"

// ServerAction requests a lifecycle operation or status inspection.
const ServerAction CommandName = "server.action"

// ServerFiles invokes contained game data file management.
const ServerFiles CommandName = "server.files"

// Command is a correlated, bounded-lifetime request.
type Command struct {
	ID         string                `json:"id"`
	Name       CommandName           `json:"name"`
	Deadline   time.Time             `json:"deadline"`
	Deployment *model.Deployment     `json:"deployment,omitempty"`
	Runtime    *model.RuntimeRequest `json:"runtime,omitempty"`
	Files      *model.FileRequest    `json:"files,omitempty"`
}

// CommandError describes a failed operation independently of transport failure.
type CommandError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *CommandError) Error() string { return e.Code + ": " + e.Message }

// CommandResponse completes exactly one request on the originating session.
type CommandResponse struct {
	ID      string                      `json:"id"`
	Receipt *model.ConfigurationReceipt `json:"receipt,omitempty"`
	Error   *CommandError               `json:"error,omitempty"`
	Status  *model.RuntimeStatus        `json:"status,omitempty"`
	Files   *model.FileResult           `json:"files,omitempty"`
}

// Enrollment returns a signed identity after consuming a bootstrap token.
type Enrollment struct {
	ID          string `json:"id"`
	Certificate []byte `json:"certificate"`
	CA          []byte `json:"ca"`
}

// Message carries versioned agent handshake and heartbeat data.
type Message struct {
	Type     string           `json:"type"`
	Protocol int              `json:"protocol"`
	SparkID  string           `json:"spark_id,omitempty"`
	Hostname string           `json:"hostname,omitempty"`
	Version  string           `json:"version,omitempty"`
	Command  *Command         `json:"command,omitempty"`
	Response *CommandResponse `json:"response,omitempty"`
	Console  *ConsoleFrame    `json:"console,omitempty"`
}

// ConsoleFrame identifies one bounded console stream message on its originating session.
type ConsoleFrame struct {
	ID       string             `json:"id"`
	ServerID string             `json:"server_id"`
	Event    model.ConsoleEvent `json:"event"`
}
