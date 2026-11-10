// Package limits defines application policies shared by validation and execution.
package limits

import "time"

// Shared limits keep validation, runtime policies, and CLI defaults consistent.
const (
	ResourceNameLength        = 100
	DescriptionLength         = 1000
	ServerMinCPU              = 0.1
	MaxCPU                    = 1024
	MinMemoryMiB              = 64
	MaxMemoryMiB              = 1 << 20
	MaxDiskMiB                = 1 << 30
	MaxServerPorts            = 32
	MinPort                   = 1
	MaxPort                   = 65535
	UsernameMin               = 3
	UsernameMax               = 64
	PasswordMin               = 15
	PasswordMax               = 1024
	DisplayNameMax            = 100
	EmailMax                  = 254
	PasswordIterations        = 600000
	PasswordKeyBytes          = 32
	PasswordAlgorithm         = "pbkdf2-sha256"
	SessionLifetime           = 12 * time.Hour
	SessionIdleTimeout        = 30 * time.Minute
	SessionTouchInterval      = time.Minute
	MaxSessionsPerUser        = 20
	LoginWindow               = time.Minute
	LoginGlobalAttempts       = 100
	LoginClientAttempts       = 10
	LoginClients              = 4096
	BackupRetentionCopies     = 1000
	BackupRetentionDays       = 36500
	LogCaptureBytes           = 64 << 10
	ConsoleFrameBytes         = 8 << 10
	ConsoleErrorBytes         = 256
	ConsoleStreamIDLength     = 128
	DocumentNodes             = 100000
	GameMediaNameLength       = 128
	GameMediaIDLength         = 64
	GamePlayers               = 1000000
	GameFields                = 32
	GameFieldNameLength       = 64
	GameFieldLabelLength      = 128
	GameFieldStringLength     = 1024
	MinimumFreshSeconds       = 15
	FreshIntervals            = 3
	DefaultGameEventRetention = 30 * 24 * time.Hour
	DefaultGameEventLimit     = 10000
	MinGameEventRetentionDays = 1
	MaxGameEventRetentionDays = 3650
	MaxGameEventLimit         = 100000
	DefaultAlertPercent       = 90
	DefaultAlertDuration      = 2 * time.Minute
	MaxAlertDuration          = 24 * time.Hour
	DefaultAlertHealth        = true
	DeliveryAttempts          = 5
	DeliveryRetryBase         = 30 * time.Second
)

// SampleFreshSeconds returns the shared lifetime of a collected container sample.
func SampleFreshSeconds(interval int64) int64 {
	return max(MinimumFreshSeconds, FreshIntervals*interval)
}
