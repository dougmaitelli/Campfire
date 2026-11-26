import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { DashboardPage } from "../../src/pages/DashboardPage";
import type * as BackupsAPI from "../../src/api/backups";
import type * as ActivityAPI from "../../src/api/activity";
import type * as ServersAPI from "../../src/api/servers";
import { listAllBackups, type CatalogBackup } from "../../src/api/backups";
import { listSchedules, type Schedule } from "../../src/api/activity";
import { listServers, type Server } from "../../src/api/servers";
import { APIError } from "../../src/api/client";
import {
  BackupState,
  RuntimeState,
  ScheduleAction,
} from "../../src/contracts/domains";

vi.mock("../../src/hooks/useAuthentication", () => ({
  useAuthentication: () => ({ session: null }),
}));
vi.mock("../../src/hooks/useMonitoring", () => ({
  useMonitoring: () => ({
    snapshot: null,
    error: null,
    loading: false,
    elapsed: 0,
  }),
}));
vi.mock("../../src/hooks/useServerRuntime", () => ({
  useServerRuntime: () => ({ statuses: {}, upsert: vi.fn() }),
}));
vi.mock("../../src/api/backups", async (original) => ({
  ...(await original<typeof BackupsAPI>()),
  listAllBackups: vi.fn(),
}));
vi.mock("../../src/api/activity", async (original) => ({
  ...(await original<typeof ActivityAPI>()),
  listSchedules: vi.fn(),
}));
vi.mock("../../src/api/servers", async (original) => ({
  ...(await original<typeof ServersAPI>()),
  listServers: vi.fn(),
}));

const server: Server = {
  id: "game",
  name: "Survival",
  description: "",
  spark_id: "node",
  ember_id: "ember",
  image: "game:latest",
  variables: {},
  cpu_limit: 1,
  memory_mb: 1024,
  disk_mb: 1024,
  ports: [],
  ember_hash: "hash",
  ember_update_available: false,
  owner_user_id: 1,
  permissions: [],
  spark_name: "Node",
  ember_name: "Game",
  game_address: "",
  status: RuntimeState.Running,
  created_at: "2026-10-25",
};
const backup: CatalogBackup = {
  id: "archive",
  display_name: "Nightly archive",
  pinned: false,
  name: "nightly.tar.gz",
  size: 100,
  modified_at: "2026-10-25T00:00:00Z",
  local: true,
  remote: false,
  state: BackupState.Available,
  can_download: false,
  can_edit: false,
  server_id: server.id,
  server_name: server.name,
  local_spark_id: "node",
  local_spark_name: "Node",
};
const schedule: Schedule = {
  id: "nightly",
  server_id: server.id,
  name: "Nightly schedule",
  action: ScheduleAction.Backup,
  cron_expression: "0 5 * * *",
  time_zone: "UTC",
  only_when_running: false,
  next_run_at: 2000000000,
  enabled: true,
  run_as_user_id: 1,
  username: "admin",
  created_at: 1,
};
beforeEach(() => {
  vi.useFakeTimers();
  vi.mocked(listServers).mockResolvedValue([server]);
  vi.mocked(listAllBackups).mockReset();
  vi.mocked(listSchedules).mockReset();
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
function summary(label: string) {
  return screen
    .getByText(label, { selector: ".metric-tile-label" })
    .closest("article")
    ?.querySelector(".metric-tile-value")?.textContent;
}
async function mount() {
  await act(async () => {
    render(<DashboardPage />);
  });
}
async function poll() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(10000);
  });
}

test.each([false, true])(
  "dashboard retains loaded summaries after refresh failure (empty: %s)",
  async (empty) => {
    vi.mocked(listAllBackups).mockResolvedValue(empty ? [] : [backup]);
    vi.mocked(listSchedules).mockResolvedValue(empty ? [] : [schedule]);
    await mount();
    const expectedBackups = empty ? "0" : "1 / 1";
    const expectedSchedules = empty ? "0 enabled" : "1 enabled";
    expect(summary("Backups")).toBe(expectedBackups);
    expect(summary("Schedules")).toBe(expectedSchedules);
    for (const failure of [
      new APIError("Refresh failed", 503),
      new TypeError("offline"),
    ]) {
      vi.mocked(listAllBackups).mockRejectedValue(failure);
      vi.mocked(listSchedules).mockRejectedValue(failure);
      await poll();
      expect(summary("Backups")).toBe(expectedBackups);
      expect(summary("Schedules")).toBe(expectedSchedules);
      expect(screen.queryByText("Summary unavailable")).toBeNull();
      if (!empty) {
        expect(
          screen.getByRole("table", { name: "Recent backups" }).textContent,
        ).toContain("Survival");
        expect(screen.getByText("Nightly schedule")).toBeTruthy();
      }
    }
    vi.mocked(listAllBackups).mockResolvedValue([]);
    vi.mocked(listSchedules).mockResolvedValue([]);
    await poll();
    expect(summary("Backups")).toBe("0");
    expect(summary("Schedules")).toBe("0 enabled");
  },
);

test("initial failed dashboard loads still show unavailable summaries", async () => {
  vi.mocked(listAllBackups).mockRejectedValue(new APIError("Unavailable", 503));
  vi.mocked(listSchedules).mockRejectedValue(new APIError("Unavailable", 503));
  await mount();
  expect(screen.getAllByText("Summary unavailable")).toHaveLength(2);
  vi.mocked(listAllBackups).mockResolvedValue([backup]);
  vi.mocked(listSchedules).mockResolvedValue([schedule]);
  await poll();
  expect(summary("Backups")).toBe("1 / 1");
  expect(summary("Schedules")).toBe("1 enabled");
});
