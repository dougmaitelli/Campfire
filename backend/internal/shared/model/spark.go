// Package model defines game server nodes and shared domain types.
package model

import "time"

// Spark is a registered game server node. Registration does not imply connectivity.
type Spark struct {
	SparkSettings
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Disabled    bool   `json:"disabled"`
	// Connected is calculated by the service and is never persisted.
	Connected            bool      `json:"connected"`
	AgentVersion         string    `json:"agent_version,omitempty"`
	LatestAgentVersion   string    `json:"latest_agent_version,omitempty"`
	AgentUpdateAvailable bool      `json:"agent_update_available"`
	Protocol             int       `json:"protocol,omitempty"`
	Capabilities         []string  `json:"capabilities,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

// SparkSettings describes administrator-configured capacity and game networking.
// Zero resource limits mean no configured limit; enforcement belongs to the runtime.
type SparkSettings struct {
	GameAddress     string           `json:"game_address"`
	CPULimit        float64          `json:"cpu_limit"`
	MemoryMB        int64            `json:"memory_mb"`
	DiskMB          int64            `json:"disk_mb"`
	PortAllocations []PortAllocation `json:"port_allocations"`
}

// PortAllocation is a pool of game ports available on one node interface.
type PortAllocation struct {
	BindIP   string `json:"bind_ip"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Protocol string `json:"protocol"`
}

// SparkChanges contains mutable fields; nil values leave stored values unchanged.
type SparkChanges struct {
	Name            *string           `json:"name"`
	Description     *string           `json:"description"`
	Disabled        *bool             `json:"disabled"`
	GameAddress     *string           `json:"game_address"`
	CPULimit        *float64          `json:"cpu_limit"`
	MemoryMB        *int64            `json:"memory_mb"`
	DiskMB          *int64            `json:"disk_mb"`
	PortAllocations *[]PortAllocation `json:"port_allocations"`
}
