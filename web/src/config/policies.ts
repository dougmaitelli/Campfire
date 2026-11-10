export const ResourceLimits = {
  NameLength: 100,
  DescriptionLength: 1000,
  ServerMinCPU: 0.1,
  MaxCPU: 1024,
  MinMemoryMiB: 64,
  MaxMemoryMiB: 1048576,
  MaxDiskMiB: 1073741824,
  MaxPorts: 32,
  MinPort: 1,
  MaxPort: 65535,
} as const;
export const AccountLimits = {
  UsernameMin: 3,
  UsernameMax: 64,
  PasswordMin: 15,
  PasswordMax: 1024,
  DisplayNameMax: 100,
  EmailMax: 254,
} as const;
export const RoleLimits = { NameLength: 64 } as const;
export const BackupRetentionLimits = { Copies: 1000, AgeDays: 36500 } as const;
export const GameLimits = {
  MediaBytes: 25 * 1024 * 1024,
  MediaNameLength: 128,
  MediaIDLength: 64,
  Players: 1000000,
  Fields: 32,
  FieldNameLength: 64,
  FieldLabelLength: 128,
  FieldStringLength: 1024,
} as const;
export const IntegrationLimits = { CardFields: 32 } as const;
export const RequestTimeout = {
  Default: 35000,
  Read: 15000,
  Archive: 30 * 60 * 1000,
} as const;
export const BuiltinRole = {
  Admin: 1,
  Creator: 2,
  User: 3,
  Owner: 4,
  Operator: 5,
} as const;
export const MonitoringPolicy = {
  MinimumFreshSeconds: 15,
  FreshIntervals: 3,
  DiskFreshSeconds: 90,
} as const;

export function sampleFreshSeconds(intervalSeconds: number): number {
  return Math.max(
    MonitoringPolicy.MinimumFreshSeconds,
    intervalSeconds * MonitoringPolicy.FreshIntervals,
  );
}
