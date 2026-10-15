package model

// ResourceUsage contains measured usage; nil values mean unavailable, never zero.
// CPU percent uses one logical CPU as 100%. Disk is logical game-data file size.
type ResourceUsage struct {
	ServerID         string   `json:"server_id"`
	State            string   `json:"state"`
	CPUPercent       *float64 `json:"cpu_percent"`
	MemoryBytes      *int64   `json:"memory_bytes"`
	DiskBytes        *int64   `json:"disk_bytes"`
	SampledAt        int64    `json:"sampled_at"`
	DiskSampledAt    int64    `json:"disk_sampled_at"`
	SampleAgeSeconds int64    `json:"sample_age_seconds"`
	DiskAgeSeconds   int64    `json:"disk_age_seconds"`
	ContainerError   string   `json:"container_error"`
	DiskError        string   `json:"disk_error"`
}
