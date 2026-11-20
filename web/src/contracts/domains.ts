export enum DiskAccounting {
  Logical = "logical",
  Project = "project",
  Mixed = "mixed",
}

export enum RuntimeState {
  NotInstalled = "not_installed",
  Installing = "installing",
  Updating = "updating",
  Reinstalling = "reinstalling",
  Starting = "starting",
  Running = "running",
  Stopping = "stopping",
  Stopped = "stopped",
  Restarting = "restarting",
  Deleting = "deleting",
  Deleted = "deleted",
  Unknown = "unknown",
  Offline = "offline",
  Created = "created",
  Paused = "paused",
  Exited = "exited",
  Dead = "dead",
  Removing = "removing",
  Missing = "missing",
  Failed = "failed",
  Error = "error",
}

export enum ConnectionState {
  Connected = "connected",
}
// Monitoring combines runtime and connectivity states. Reuse the actual enum
// members so an observed runtime state can be compared with its runtime response.
export const ResourceState = { ...RuntimeState, ...ConnectionState } as const;
export type ResourceState = RuntimeState | ConnectionState;

export enum ServerAction {
  Install = "install",
  Update = "update",
  Reinstall = "reinstall",
  Start = "start",
  Stop = "stop",
  Restart = "restart",
  Kill = "kill",
  Delete = "delete",
  Status = "status",
  Logs = "logs",
  Command = "command",
  Metrics = "metrics",
  Backup = "backup",
  Restore = "restore",
  Transfer = "transfer",
}
export type LifecycleAction = Exclude<
  ServerAction,
  | ServerAction.Delete
  | ServerAction.Status
  | ServerAction.Logs
  | ServerAction.Command
  | ServerAction.Metrics
  | ServerAction.Transfer
>;
// Schedules select a subset of the same server actions, not a second domain.
export const ScheduleAction = {
  Start: ServerAction.Start,
  Stop: ServerAction.Stop,
  Restart: ServerAction.Restart,
  Kill: ServerAction.Kill,
  Backup: ServerAction.Backup,
  Command: ServerAction.Command,
} as const;
export type ScheduleAction =
  (typeof ScheduleAction)[keyof typeof ScheduleAction];
export enum OperationState {
  Queued = "queued",
  Running = "running",
  Succeeded = "succeeded",
  Failed = "failed",
  Interrupted = "interrupted",
}
export enum RuntimeErrorCode {
  OperationFailed = "operation_failed",
  OperationInterrupted = "operation_interrupted",
  DockerFailed = "docker_operation_failed",
  InstallerFailed = "installer_failed",
  QuotaUnavailable = "disk_quota_unavailable",
}
export enum ExecutionState {
  Pending = "pending",
  Queued = "queued",
  Dispatching = "dispatching",
  Running = "running",
  Succeeded = "succeeded",
  Failed = "failed",
  Interrupted = "interrupted",
  Cancelled = "cancelled",
  Skipped = "skipped",
  Accepted = "accepted",
  Completed = "completed",
}
export enum AuditOutcome {
  Pending = "pending",
  Accepted = "accepted",
  Succeeded = "succeeded",
  Failed = "failed",
  Interrupted = "interrupted",
  Cancelled = "cancelled",
  Skipped = "skipped",
}
export enum ActivityFilterKind {
  Game = "game",
  Operations = "operations",
  Alert = "alert",
}
export enum ActivityEntryKind {
  Game = "game",
  Operation = "operation",
  Job = "job",
  Alert = "alert",
  Notification = "notification",
}
export enum NetworkProtocol {
  TCP = "tcp",
  UDP = "udp",
}
export enum RoleScope {
  Platform = "platform",
  Server = "server",
}
export enum BackupMode {
  Full = "full",
  Savedata = "savedata",
}
export enum BackupStorage {
  Local = "local",
  S3 = "s3",
}
export enum BackupState {
  Available = "available",
  Syncing = "syncing",
  Deleting = "deleting",
  Failed = "failed",
  Error = "error",
  Unavailable = "unavailable",
}
export enum ReinstallMode {
  Keep = "keep",
  Clean = "clean",
}
export enum FileKind {
  File = "file",
  Directory = "directory",
  Unsupported = "unsupported",
}
export enum ConfigParser {
  Properties = "properties",
  JSON = "json",
  YAML = "yaml",
  INI = "ini",
  File = "file",
}
export enum StartupShell {
  Sh = "sh",
  Bash = "bash",
  Ash = "ash",
}
export enum FileOperation {
  List = "list",
  Read = "read",
  Write = "write",
  Mkdir = "mkdir",
  Rename = "rename",
  Delete = "delete",
  Chmod = "chmod",
  Stat = "stat",
  UploadBegin = "upload.begin",
  UploadChunk = "upload.chunk",
  UploadCommit = "upload.commit",
  UploadCancel = "upload.cancel",
  Compress = "compress",
  CompressStart = "compress.start",
  CompressStatus = "compress.status",
  CompressCancel = "compress.cancel",
  Extract = "extract",
  ExtractStart = "extract.start",
  ExtractStatus = "extract.status",
  ExtractResolve = "extract.resolve",
  ExtractCancel = "extract.cancel",
}
export enum CompressionState {
  Running = "running",
  Cancelling = "cancelling",
  Completed = "completed",
  Cancelled = "cancelled",
  Failed = "failed",
}
export enum WorkPhase {
  Preparing = "preparing",
  Archiving = "archiving",
  Extracting = "extracting",
  Validating = "validating",
  Uploading = "uploading",
  Downloading = "downloading",
  Restoring = "restoring",
  Committing = "committing",
}
export enum ExtractionState {
  Running = "running",
  Waiting = "waiting",
  Completed = "completed",
  Cancelled = "cancelled",
  Expired = "expired",
  Failed = "failed",
}
export enum ExtractionAction {
  Skip = "skip",
  Rename = "rename",
  Overwrite = "overwrite",
  Merge = "merge",
  Extract = "extract",
}
export enum ExtractionOutcomeStatus {
  Pending = "pending",
  Extracted = "extracted",
  Merged = "merged",
  Overwritten = "overwritten",
  Renamed = "renamed",
  Skipped = "skipped",
  Failed = "failed",
  Cancelled = "cancelled",
}
export enum BrowserPublicationStatus {
  Uploaded = "uploaded",
  Unconfirmed = "unconfirmed",
}
export enum FileUploadPhase {
  Uploading = "uploading",
  Saving = "saving",
  Cancelling = "cancelling",
}
export enum VariableType {
  String = "string",
  Integer = "integer",
  Numeric = "numeric",
  Boolean = "boolean",
}
export enum VariableFormat {
  URL = "url",
}
export enum NotificationTarget {
  Account = "account",
  Server = "server",
  Spark = "spark",
}
export enum NotificationKind {
  Backup = "backup",
  Health = "health",
  Schedule = "schedule",
  Test = "test",
  Alert = "alert",
  Recovery = "recovery",
}
export enum NotificationSeverity {
  Info = "info",
  Warning = "warning",
  Failure = "failure",
  Success = "success",
}
export enum NotificationCategory {
  Game = "game",
  Power = "power",
  ConsoleAlert = "console_alert",
  Backup = "backup",
}
export enum IntegrationCardState {
  Disabled = "disabled",
  Pending = "pending",
  Published = "published",
  Uncertain = "uncertain",
  Failed = "failed",
}
export enum ResourceKind {
  Server = "server",
  Spark = "spark",
  Ember = "ember",
  User = "user",
  Role = "role",
  Schedule = "schedule",
}
export enum ServerTab {
  Console = "console",
  Files = "files",
  Backups = "backups",
  Activity = "activity",
}
export enum GameStateStatus {
  Fresh = "fresh",
  Stale = "stale",
  Unavailable = "unavailable",
}
export enum JobType {
  Copy = "copy",
  Transfer = "transfer",
}
export enum JobAction {
  Retry = "retry",
  Cancel = "cancel",
}
export enum JobStatus {
  Pending = "pending",
  Running = "running",
  Failed = "failed",
  Cancelling = "cancelling",
  Completed = "completed",
  Cancelled = "cancelled",
}
export enum CrashReason {
  Unknown = "unknown",
  OutOfMemory = "out_of_memory",
}

export enum JobStep {
  Preparing = "preparing",
  Exporting = "exporting",
  Copying = "copying",
  Importing = "importing",
  Committing = "committing",
  Cleanup = "cleanup",
}
