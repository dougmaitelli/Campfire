package model

// PermissionID identifies a validated platform or server grant.
type PermissionID string

// RoleScope separates platform roles from server access roles.
type RoleScope string

// Role scopes separate platform administration from per-server access.
const (
	RoleScopePlatform RoleScope = "platform"
	RoleScopeServer   RoleScope = "server"
)

// NetworkProtocol identifies an allocatable game transport.
type NetworkProtocol string

// Network protocols retain the Docker and allocation transport names.
const (
	NetworkTCP NetworkProtocol = "tcp"
	NetworkUDP NetworkProtocol = "udp"
)

// Valid reports whether the protocol can be allocated.
func (p NetworkProtocol) Valid() bool { return p == NetworkTCP || p == NetworkUDP }

// BackupStorage identifies the requested archive location.
type BackupStorage string

// Backup storage values select local, remote, or all archive locations.
const (
	BackupStorageLocal BackupStorage = "local"
	BackupStorageS3    BackupStorage = "s3"
	BackupStorageAll   BackupStorage = "all"
)

// BackupState describes the archive catalog view.
type BackupState string

// Backup states describe availability and catalog maintenance.
const (
	BackupAvailable   BackupState = "available"
	BackupUnavailable BackupState = "unavailable"
	BackupError       BackupState = "error"
	BackupSyncing     BackupState = "syncing"
	BackupDeleting    BackupState = "deleting"
	BackupFailed      BackupState = "failed"
)

// ReinstallMode selects whether installation retains existing game files.
type ReinstallMode string

// Reinstall modes determine whether game files are preserved.
const (
	ReinstallKeep  ReinstallMode = "keep"
	ReinstallClean ReinstallMode = "clean"
)

// Valid accepts the default mode as well as explicit retention choices.
func (m ReinstallMode) Valid() bool { return m == "" || m == ReinstallKeep || m == ReinstallClean }

// FileKind classifies entries without following links.
type FileKind string

// File kinds identify regular files, directories, and unsupported entries.
const (
	FileKindFile        FileKind = "file"
	FileKindDirectory   FileKind = "directory"
	FileKindUnsupported FileKind = "unsupported"
)

// FileErrorCode identifies a safe file-management failure.
type FileErrorCode string

// File errors are safe to expose through management transports.
const (
	FileInvalid               FileErrorCode = "invalid_file"
	FileNotFound              FileErrorCode = "file_not_found"
	FileConflict              FileErrorCode = "file_conflict"
	FileBusy                  FileErrorCode = "file_busy"
	FileLimitExceeded         FileErrorCode = "file_limit"
	FileIO                    FileErrorCode = "file_io"
	FileUnsupportedCapability FileErrorCode = "unsupported_capability"
)

// VariableType defines the value accepted by an Ember validation rule.
type VariableType string

// VariableFormat adds a semantic constraint to a string variable.
type VariableFormat string

// Variable rules use the Ember validation vocabulary.
const (
	VariableString    VariableType   = "string"
	VariableInteger   VariableType   = "integer"
	VariableNumeric   VariableType   = "numeric"
	VariableBoolean   VariableType   = "boolean"
	VariableFormatURL VariableFormat = "url"
)

// GameStateStatus describes the availability of a game-state snapshot.
type GameStateStatus string

// Game-state status describes snapshot freshness.
const (
	GameStateFresh       GameStateStatus = "fresh"
	GameStateStale       GameStateStatus = "stale"
	GameStateUnavailable GameStateStatus = "unavailable"
)

// JobRole identifies a Spark's side of a saved transfer.
type JobRole string

// Transfer roles distinguish the source and destination Spark.
const (
	JobSource JobRole = "source"
	JobTarget JobRole = "target"
)
