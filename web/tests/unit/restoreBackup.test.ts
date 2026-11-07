import { afterEach, expect, test, vi } from "vitest";
import { restoreBackup } from "../../src/components/servers/restoreBackup";
import {
  serverAction,
  serverStatus,
  type RuntimeStatus,
} from "../../src/api/runtime";
import {
  OperationState,
  RuntimeState,
  ServerAction,
} from "../../src/contracts/domains";

vi.mock("../../src/api/runtime", () => ({
  serverAction: vi.fn(),
  serverStatus: vi.fn(),
}));
afterEach(() => {
  vi.resetAllMocks();
  vi.useRealTimers();
});

function status(
  action: "stop" | "restore" | "start",
  busy = false,
): RuntimeStatus {
  return {
    server_id: "game",
    state: action === "start" ? RuntimeState.Running : RuntimeState.Stopped,
    operation_id: action,
    operation: action,
    operation_state: busy ? OperationState.Running : OperationState.Succeeded,
    ready: true,
    busy,
    disk_quota_enforced: false,
  };
}

function fixture(running = true) {
  vi.mocked(serverStatus).mockResolvedValueOnce({
    ...status("start"),
    state: running ? RuntimeState.Running : RuntimeState.Stopped,
  });
  vi.mocked(serverAction).mockImplementation(async (_id, action) =>
    status(action as "stop" | "restore" | "start"),
  );
  return new AbortController();
}

test("restore waits for stop and restore to finish before starting", async () => {
  vi.useFakeTimers();
  const controller = fixture();
  vi.mocked(serverAction).mockImplementation(async (_id, action) =>
    status(action as "stop" | "restore" | "start", true),
  );
  vi.mocked(serverStatus)
    .mockResolvedValueOnce(status("stop"))
    .mockResolvedValueOnce(status("restore"))
    .mockResolvedValueOnce(status("start"));
  const result = restoreBackup(
    "game",
    "snapshot.tar.gz",
    { stop: true, restart: true },
    controller.signal,
    vi.fn(),
    vi.fn(),
  );
  await vi.advanceTimersByTimeAsync(0);
  expect(serverAction).toHaveBeenCalledTimes(1);
  expect(serverAction).toHaveBeenLastCalledWith(
    "game",
    ServerAction.Stop,
    undefined,
  );
  await vi.advanceTimersByTimeAsync(1000);
  expect(serverAction).toHaveBeenLastCalledWith(
    "game",
    ServerAction.Restore,
    "snapshot.tar.gz",
  );
  await vi.advanceTimersByTimeAsync(1000);
  expect(serverAction).toHaveBeenLastCalledWith(
    "game",
    ServerAction.Start,
    undefined,
  );
  await vi.advanceTimersByTimeAsync(1000);
  expect(await result).toMatchObject(status("start"));
});

test.each([false, true])(
  "an already-stopped server stays stopped with restart %s",
  async (restart) => {
    const controller = fixture(false);
    await restoreBackup(
      "game",
      "snapshot.tar.gz",
      { stop: true, restart },
      controller.signal,
      vi.fn(),
      vi.fn(),
    );
    expect(serverAction).toHaveBeenCalledExactlyOnceWith(
      "game",
      ServerAction.Restore,
      "snapshot.tar.gz",
    );
  },
);

test("restart can be declined for a running server", async () => {
  const controller = fixture();
  await restoreBackup(
    "game",
    "snapshot.tar.gz",
    { stop: true, restart: false },
    controller.signal,
    vi.fn(),
    vi.fn(),
  );
  expect(vi.mocked(serverAction).mock.calls.map((call) => call[1])).toEqual([
    "stop",
    "restore",
  ]);
});

test.each(["stop", "restore", "start"] as const)(
  "a failed %s stops the sequence",
  async (failed) => {
    const controller = fixture();
    vi.mocked(serverAction).mockImplementation(async (_id, action) => ({
      ...status(action as "stop" | "restore" | "start"),
      ...(action === failed
        ? { error: `${failed} failed`, operation_state: OperationState.Failed }
        : {}),
    }));
    await expect(
      restoreBackup(
        "game",
        "snapshot.tar.gz",
        { stop: true, restart: true },
        controller.signal,
        vi.fn(),
        vi.fn(),
      ),
    ).rejects.toThrow(`${failed} failed`);
    expect(vi.mocked(serverAction).mock.calls.map((call) => call[1])).toEqual(
      ["stop", "restore", "start"].slice(
        0,
        ["stop", "restore", "start"].indexOf(failed) + 1,
      ),
    );
  },
);

test("a server that starts after confirmation requires fresh consent to stop", async () => {
  const controller = fixture();
  await expect(
    restoreBackup(
      "game",
      "snapshot.tar.gz",
      { stop: false, restart: true },
      controller.signal,
      vi.fn(),
      vi.fn(),
    ),
  ).rejects.toThrow("confirm stopping");
  expect(serverAction).not.toHaveBeenCalled();
});

test("an aborted sequence never issues the next command", async () => {
  const controller = fixture();
  vi.mocked(serverAction).mockImplementationOnce(async () => {
    controller.abort();
    return status("stop");
  });
  await expect(
    restoreBackup(
      "game",
      "snapshot.tar.gz",
      { stop: true, restart: true },
      controller.signal,
      vi.fn(),
      vi.fn(),
    ),
  ).rejects.toThrow();
  expect(serverAction).toHaveBeenCalledTimes(1);
});

test("a different operation during polling stops the restore sequence", async () => {
  vi.useFakeTimers();
  const controller = fixture();
  vi.mocked(serverAction).mockResolvedValueOnce(status("stop", true));
  vi.mocked(serverStatus).mockResolvedValueOnce({
    ...status("start"),
    operation_id: "another-operation",
  });
  const result = expect(
    restoreBackup(
      "game",
      "snapshot.tar.gz",
      { stop: true, restart: true },
      controller.signal,
      vi.fn(),
      vi.fn(),
    ),
  ).rejects.toThrow("Another operation");
  await vi.advanceTimersByTimeAsync(1000);
  await result;
  expect(serverAction).toHaveBeenCalledTimes(1);
});
