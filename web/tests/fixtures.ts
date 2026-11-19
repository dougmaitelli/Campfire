import { RuntimeState } from "../src/contracts/domains";
import type { RuntimeStatus } from "../src/api/runtime";

export function runtimeStatus(
  overrides: Partial<RuntimeStatus> = {},
): RuntimeStatus {
  return {
    server_id: "server/one",
    state: RuntimeState.Stopped,
    ready: true,
    busy: false,
    file_system_busy: false,
    disk_quota_enforced: false,
    ...overrides,
  };
}
