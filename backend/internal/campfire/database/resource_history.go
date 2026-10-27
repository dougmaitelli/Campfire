package database

import "campfire/internal/shared/model"

// ResourceAggregate keeps weighted sums and separate counts for unavailable metrics.
type ResourceAggregate struct {
	BucketStart     int64   `gorm:"primaryKey;autoIncrement:false;index"`
	SampleCount     int64   `gorm:"not null"`
	CPUCount        int64   `gorm:"not null"`
	CPUSum          float64 `gorm:"not null"`
	CPUPeak         float64 `gorm:"not null"`
	MemoryCount     int64   `gorm:"not null"`
	MemorySum       float64 `gorm:"not null"`
	MemoryPeak      int64   `gorm:"not null"`
	CPULimitSum     float64 `gorm:"not null"`
	MemoryLimitSum  float64 `gorm:"not null"`
	LatestAt        int64   `gorm:"not null"`
	CPUUnlimited    int64   `gorm:"not null"`
	MemoryUnlimited int64   `gorm:"not null"`
}

// ServerResourceBucket is one indexed, five-minute resource rollup.
type ServerResourceBucket struct {
	ServerID          string `gorm:"type:text;primaryKey"`
	Server            Server `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE"`
	ResourceAggregate `gorm:"embedded"`
}

// SparkResourceBucket retains combined game-server measurements for a node.
type SparkResourceBucket struct {
	SparkID           string `gorm:"type:text;primaryKey"`
	Spark             Spark  `gorm:"foreignKey:SparkID;constraint:OnDelete:CASCADE"`
	ResourceAggregate `gorm:"embedded"`
}

// ServerResourceSample retains usage independently of browser connections.
type ServerResourceSample struct {
	ServerID    string              `gorm:"type:text;primaryKey"`
	Server      Server              `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE"`
	SparkID     string              `gorm:"type:text;not null"`
	CollectedAt int64               `gorm:"primaryKey;autoIncrement:false;index"`
	ReceivedAt  int64               `gorm:"not null"`
	CPULimit    float64             `gorm:"not null"`
	MemoryMB    int64               `gorm:"not null"`
	DiskMB      int64               `gorm:"not null"`
	Usage       model.ResourceUsage `gorm:"serializer:json;type:text;not null"`
}

// SparkResourceSample stores combined game-server usage, not host-wide usage.
type SparkResourceSample struct {
	SparkID     string              `gorm:"type:text;primaryKey"`
	Spark       Spark               `gorm:"foreignKey:SparkID;constraint:OnDelete:CASCADE"`
	CollectedAt int64               `gorm:"primaryKey;autoIncrement:false;index"`
	ReceivedAt  int64               `gorm:"not null"`
	CPULimit    float64             `gorm:"not null"`
	MemoryMB    int64               `gorm:"not null"`
	DiskMB      int64               `gorm:"not null"`
	Usage       model.ResourceUsage `gorm:"serializer:json;type:text;not null"`
}
