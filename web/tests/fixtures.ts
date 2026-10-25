import type { RuntimeStatus } from "../src/api/runtime";

export function runtimeStatus(
  overrides: Partial<RuntimeStatus> = {},
): RuntimeStatus {
  return {
    server_id: "server/one",
    state: "stopped",
    ready: true,
    busy: false,
    disk_quota_enforced: false,
    ...overrides,
  };
}
