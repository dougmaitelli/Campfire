import { expect, test, vi } from "vitest";
import { manageFiles } from "../../src/api/files";
import { serverStatus } from "../../src/api/runtime";
import { isResourceUsage } from "../../src/api/monitoring";
import { listNotifications } from "../../src/api/notifications";
import { runtimeStatus } from "../fixtures";

test.each(["backup", "health", "schedule", "alert", "recovery", "test"])(
  "notification responses retain the %s message kind",
  async (kind) => {
    const notification = {
      id: 1,
      target_type: "account",
      target_id: "1",
      kind,
      severity: "info",
      title: "Example",
      body: "Example message",
      created_at: 1,
      read_at: 0,
    };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json([notification]),
    );
    await expect(
      listNotifications(new AbortController().signal),
    ).resolves.toEqual([notification]);
  },
);

test("monitoring accepts Spark totals without a server state and rejects unknown states", () => {
  const usage = {
    server_id: "",
    state: "",
    cpu_percent: 0,
    memory_bytes: 0,
    disk_bytes: 0,
    sampled_at: 1,
    disk_sampled_at: 1,
    sample_age_seconds: 0,
    disk_age_seconds: 0,
    container_error: "",
    disk_error: "",
    disk_quota_enforced: false,
    disk_quota_error: "",
  };
  expect(isResourceUsage(usage)).toBe(true);
  expect(isResourceUsage({ ...usage, state: "connected" })).toBe(true);
  expect(isResourceUsage({ ...usage, state: "running" })).toBe(true);
  expect(isResourceUsage({ ...usage, state: "misspelled" })).toBe(false);
});

test.each(["file", "directory", "unsupported"])(
  "file responses accept the %s entry kind",
  async (kind) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        server_id: "server/one",
        path: "/",
        entries: [
          {
            name: "example",
            kind,
            size: 0,
            modified_at: "2026-01-01T00:00:00Z",
          },
        ],
      }),
    );
    await expect(
      manageFiles("server/one", { operation: "list", path: "/" }),
    ).resolves.toMatchObject({ path: "/" });
  },
);

test.each([{ kind: ["file"] }, { kind: "unknown" }, { kind: null }])(
  "file responses reject malformed entry kind $kind",
  async ({ kind }) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        server_id: "server/one",
        path: "/",
        entries: [
          {
            name: "example",
            kind,
            size: 0,
            modified_at: "2026-01-01T00:00:00Z",
          },
        ],
      }),
    );
    await expect(
      manageFiles("server/one", { operation: "list", path: "/" }),
    ).rejects.toThrow("Unexpected file response");
  },
);

test.each([true, false, undefined])(
  "runtime responses accept filesystem busy state %s, including older Sparks",
  async (fileSystemBusy) => {
    const status = {
      ...runtimeStatus({ server_id: "server" }),
      ...(fileSystemBusy === undefined
        ? {}
        : { file_system_busy: fileSystemBusy }),
    };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(status));
    await expect(
      serverStatus("server", new AbortController().signal),
    ).resolves.toEqual(status);
  },
);

test.each(["false", 0, null])(
  "runtime responses reject malformed filesystem busy state %s",
  async (fileSystemBusy) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        ...runtimeStatus({ server_id: "server" }),
        file_system_busy: fileSystemBusy,
      }),
    );
    await expect(
      serverStatus("server", new AbortController().signal),
    ).rejects.toThrow("Unexpected runtime response");
  },
);

test("runtime responses reject unknown operation states", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    Response.json({ ...runtimeStatus(), operation_state: "misspelled" }),
  );
  await expect(
    serverStatus("server/one", new AbortController().signal),
  ).rejects.toThrow("Unexpected runtime response");
});
