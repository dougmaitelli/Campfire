package model

// ResourceHistoryUsage contains only the metrics retained for graphing.
type ResourceHistoryUsage struct {
	CPUPercent  *float64 `json:"cpu_percent"`
	MemoryBytes *int64   `json:"memory_bytes"`
}

// ResourceSample preserves measurements and the limits in effect when collected.
type ResourceSample struct {
	SampleCount   int64                `json:"sample_count"`
	CPUSamples    int64                `json:"cpu_samples"`
	MemorySamples int64                `json:"memory_samples"`
	CPUPeak       *float64             `json:"cpu_peak"`
	MemoryPeak    *int64               `json:"memory_peak"`
	CollectedAt   int64                `json:"collected_at"`
	ReceivedAt    int64                `json:"received_at"`
	CPULimit      float64              `json:"cpu_limit"`
	MemoryMB      int64                `json:"memory_mb"`
	Usage         ResourceHistoryUsage `json:"usage"`
}

// ResourceTarget is monitoring inventory without Ember definitions or secrets.
type ResourceTarget struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	SparkID   string  `json:"spark_id"`
	SparkName string  `json:"spark_name"`
	CPULimit  float64 `json:"cpu_limit"`
	MemoryMB  int64   `json:"memory_mb"`
	DiskMB    int64   `json:"disk_mb"`
}

// ResourceHistory is a bounded series of real samples, with missing values kept null.
type ResourceHistory struct {
	From            int64            `json:"from"`
	To              int64            `json:"to"`
	IntervalSeconds int64            `json:"interval_seconds"`
	Samples         []ResourceSample `json:"samples"`
}
