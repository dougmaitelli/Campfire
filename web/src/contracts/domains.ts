export const RuntimeState = {
  NotInstalled: "not_installed",
  Installing: "installing",
  Updating: "updating",
  Reinstalling: "reinstalling",
  Starting: "starting",
  Running: "running",
  Stopping: "stopping",
  Stopped: "stopped",
  Restarting: "restarting",
  Deleting: "deleting",
  Deleted: "deleted",
  Unknown: "unknown",
  Offline: "offline",
  Created: "created",
  Paused: "paused",
  Exited: "exited",
  Dead: "dead",
  Removing: "removing",
  Missing: "missing",
  Failed: "failed",
  Error: "error",
} as const;
export type RuntimeState = (typeof RuntimeState)[keyof typeof RuntimeState];

export const ResourceState = {
  ...RuntimeState,
  Connected: "connected",
} as const;
export type ResourceState = (typeof ResourceState)[keyof typeof ResourceState];

export const ServerAction = {
  Install: "install",
  Update: "update",
  Reinstall: "reinstall",
  Start: "start",
  Stop: "stop",
  Restart: "restart",
  Kill: "kill",
  Delete: "delete",
  Status: "status",
  Logs: "logs",
  Command: "command",
  Metrics: "metrics",
  Backup: "backup",
  Restore: "restore",
  Transfer: "transfer",
} as const;
export type ServerAction = (typeof ServerAction)[keyof typeof ServerAction];
export type LifecycleAction = Exclude<
  ServerAction,
  "delete" | "status" | "logs" | "command" | "metrics" | "transfer"
>;
export const ScheduleAction = {
  Start: ServerAction.Start,
  Stop: ServerAction.Stop,
  Restart: ServerAction.Restart,
  Kill: ServerAction.Kill,
  Backup: ServerAction.Backup,
  Command: ServerAction.Command,
} as const;
export type ScheduleAction = Extract<
  ServerAction,
  "start" | "stop" | "restart" | "kill" | "backup" | "command"
>;

export const OperationState = {
  Queued: "queued",
  Running: "running",
  Succeeded: "succeeded",
  Failed: "failed",
  Interrupted: "interrupted",
} as const;
export type OperationState =
  (typeof OperationState)[keyof typeof OperationState];

export const RuntimeErrorCode = {
  OperationFailed: "operation_failed",
  OperationInterrupted: "operation_interrupted",
  DockerFailed: "docker_operation_failed",
  InstallerFailed: "installer_failed",
  QuotaUnavailable: "disk_quota_unavailable",
} as const;
export type RuntimeErrorCode =
  (typeof RuntimeErrorCode)[keyof typeof RuntimeErrorCode];

export const ExecutionState = {
  Pending: "pending",
  Queued: "queued",
  Dispatching: "dispatching",
  Running: "running",
  Succeeded: "succeeded",
  Failed: "failed",
  Interrupted: "interrupted",
  Cancelled: "cancelled",
  Skipped: "skipped",
  Accepted: "accepted",
  Completed: "completed",
} as const;
export type ExecutionState =
  (typeof ExecutionState)[keyof typeof ExecutionState];

export const AuditOutcome = {
  Pending: "pending",
  Accepted: "accepted",
  Succeeded: "succeeded",
  Failed: "failed",
  Interrupted: "interrupted",
  Cancelled: "cancelled",
  Skipped: "skipped",
} as const;
export type AuditOutcome = (typeof AuditOutcome)[keyof typeof AuditOutcome];

export const NetworkProtocol = { TCP: "tcp", UDP: "udp" } as const;
export type NetworkProtocol =
  (typeof NetworkProtocol)[keyof typeof NetworkProtocol];
export const RoleScope = { Platform: "platform", Server: "server" } as const;
export type RoleScope = (typeof RoleScope)[keyof typeof RoleScope];
export const BackupMode = { Full: "full", Savedata: "savedata" } as const;
export type BackupMode = (typeof BackupMode)[keyof typeof BackupMode];
export const BackupStorage = { Local: "local", S3: "s3" } as const;
export type BackupStorage = (typeof BackupStorage)[keyof typeof BackupStorage];
export const BackupState = {
  Available: "available",
  Syncing: "syncing",
  Deleting: "deleting",
  Failed: "failed",
  Error: "error",
  Unavailable: "unavailable",
} as const;
export type BackupState = (typeof BackupState)[keyof typeof BackupState];
export const ReinstallMode = { Keep: "keep", Clean: "clean" } as const;
export type ReinstallMode = (typeof ReinstallMode)[keyof typeof ReinstallMode];
export const FileKind = {
  File: "file",
  Directory: "directory",
  Unsupported: "unsupported",
} as const;
export type FileKind = (typeof FileKind)[keyof typeof FileKind];
export const ConfigParser = {
  Properties: "properties",
  JSON: "json",
  YAML: "yaml",
  INI: "ini",
  File: "file",
} as const;
export type ConfigParser = (typeof ConfigParser)[keyof typeof ConfigParser];
export const FileOperation = {
  List: "list",
  Read: "read",
  Write: "write",
  Mkdir: "mkdir",
  Rename: "rename",
  Delete: "delete",
  Chmod: "chmod",
  UploadBegin: "upload.begin",
  UploadChunk: "upload.chunk",
  UploadCommit: "upload.commit",
  UploadCancel: "upload.cancel",
  Compress: "compress",
  Extract: "extract",
} as const;
export type FileOperation = (typeof FileOperation)[keyof typeof FileOperation];
export const VariableType = {
  String: "string",
  Integer: "integer",
  Numeric: "numeric",
  Boolean: "boolean",
} as const;
export type VariableType = (typeof VariableType)[keyof typeof VariableType];
export const VariableFormat = { URL: "url" } as const;
export type VariableFormat =
  (typeof VariableFormat)[keyof typeof VariableFormat];

export const NotificationTarget = {
  Account: "account",
  Server: "server",
  Spark: "spark",
} as const;
export type NotificationTarget =
  (typeof NotificationTarget)[keyof typeof NotificationTarget];
export const NotificationKind = {
  Backup: "backup",
  Health: "health",
  Schedule: "schedule",
  Test: "test",
  Alert: "alert",
  Recovery: "recovery",
} as const;
export type NotificationKind =
  (typeof NotificationKind)[keyof typeof NotificationKind];
export const NotificationSeverity = {
  Info: "info",
  Warning: "warning",
  Failure: "failure",
  Success: "success",
} as const;
export type NotificationSeverity =
  (typeof NotificationSeverity)[keyof typeof NotificationSeverity];
export const ResourceKind = {
  Server: "server",
  Spark: "spark",
  Ember: "ember",
  User: "user",
  Role: "role",
  Schedule: "schedule",
} as const;
export type ResourceKind = (typeof ResourceKind)[keyof typeof ResourceKind];
export const ServerTab = {
  Console: "console",
  Files: "files",
  Backups: "backups",
  Activity: "activity",
} as const;
export type ServerTab = (typeof ServerTab)[keyof typeof ServerTab];
export const GameStateStatus = {
  Fresh: "fresh",
  Stale: "stale",
  Unavailable: "unavailable",
} as const;
export type GameStateStatus =
  (typeof GameStateStatus)[keyof typeof GameStateStatus];
export const JobType = { Copy: "copy", Transfer: "transfer" } as const;
export type JobType = (typeof JobType)[keyof typeof JobType];
export const JobStatus = {
  Pending: "pending",
  Running: "running",
  Failed: "failed",
  Cancelling: "cancelling",
  Completed: "completed",
  Cancelled: "cancelled",
} as const;
export type JobStatus = (typeof JobStatus)[keyof typeof JobStatus];
export const JobStep = {
  Preparing: "preparing",
  Exporting: "exporting",
  Copying: "copying",
  Importing: "importing",
  Committing: "committing",
  Cleanup: "cleanup",
} as const;
export type JobStep = (typeof JobStep)[keyof typeof JobStep];
