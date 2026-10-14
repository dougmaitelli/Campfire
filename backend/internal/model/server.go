package model

import "time"

// Server is a saved game server configuration; runtime state is not persisted.
type Server struct {
	ID string `json:"id"`
	ServerConfiguration
	SparkName   string    `json:"spark_name"`
	EmberName   string    `json:"ember_name"`
	GameAddress string    `json:"game_address"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// ServerConfiguration contains the operator-configured placement and reservations.
type ServerConfiguration struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	SparkID     string            `json:"spark_id"`
	EmberID     string            `json:"ember_id"`
	Image       string            `json:"image"`
	Variables   map[string]string `json:"variables"`
	CPULimit    float64           `json:"cpu_limit"`
	MemoryMB    int64             `json:"memory_mb"`
	DiskMB      int64             `json:"disk_mb"`
	Ports       []ServerPort      `json:"ports"`
}

// ServerPort reserves a host port and maps it to a container port.
type ServerPort struct {
	BindIP        string `json:"bind_ip"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}
