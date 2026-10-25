package model

import (
	"maps"
	"slices"
	"time"
)

// SameRuntimeConfiguration ignores display metadata when comparing saved settings.
func SameRuntimeConfiguration(a, b ServerConfiguration) bool {
	return a.SparkID == b.SparkID && a.EmberID == b.EmberID && a.Image == b.Image && a.StartupCommand == b.StartupCommand && a.StartupOverride == b.StartupOverride &&
		a.CPULimit == b.CPULimit && a.MemoryMB == b.MemoryMB && a.DiskMB == b.DiskMB && a.GameIntegration == b.GameIntegration &&
		maps.Equal(a.Variables, b.Variables) && slices.Equal(a.Ports, b.Ports)
}

// Server is a saved game server configuration; runtime state is not persisted.
type Server struct {
	ID          string   `json:"id"`
	OwnerUserID int64    `json:"owner_user_id"`
	Permissions []string `json:"permissions,omitempty"`
	ServerConfiguration
	SparkName            string            `json:"spark_name"`
	EmberName            string            `json:"ember_name"`
	EmberArtURL          string            `json:"ember_art_url,omitempty"`
	EmberHash            string            `json:"ember_hash"`
	EmberUpdateAvailable bool              `json:"ember_update_available"`
	GameAddress          string            `json:"game_address"`
	PortDescriptions     map[string]string `json:"port_descriptions,omitempty"`
	Status               string            `json:"status"`
	CreatedAt            time.Time         `json:"created_at"`
}

// ServerConfiguration contains the operator-configured placement and reservations.
type ServerConfiguration struct {
	StartupOverride string            `json:"startup_override,omitempty"`
	GameIntegration bool              `json:"game_integration"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	SparkID         string            `json:"spark_id"`
	EmberID         string            `json:"ember_id"`
	Image           string            `json:"image"`
	StartupCommand  string            `json:"startup_command,omitempty"`
	Variables       map[string]string `json:"variables"`
	CPULimit        float64           `json:"cpu_limit"`
	MemoryMB        int64             `json:"memory_mb"`
	DiskMB          int64             `json:"disk_mb"`
	Ports           []ServerPort      `json:"ports"`
}

// ServerPort reserves a host port and maps it to a container port.
type ServerPort struct {
	BindIP        string `json:"bind_ip"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}
