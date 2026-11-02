import { act, renderHook } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { listServerStatuses, type RuntimeStatus } from "../../src/api/runtime";
import { useServerRuntime } from "../../src/hooks/useServerRuntime";
import { RuntimeState } from "../../src/contracts/domains";

vi.mock("../../src/api/runtime", () => ({ listServerStatuses: vi.fn() }));

test("fleet polling uses one request and preserves a newer action response", async () => {
  let complete!: (statuses: RuntimeStatus[]) => void;
  vi.mocked(listServerStatuses).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  const ids = Array.from({ length: 100 }, (_, index) => `server-${index}`);
  const { result, unmount } = renderHook(() => useServerRuntime(ids));
  expect(listServerStatuses).toHaveBeenCalledOnce();
  const current: RuntimeStatus = {
    server_id: ids[0]!,
    state: RuntimeState.Starting,
    busy: true,
    ready: false,
    disk_quota_enforced: false,
  };
  act(() => result.current.upsert(current));
  await act(async () => {
    complete([{ ...current, state: RuntimeState.Stopped, busy: false }]);
  });
  expect(result.current.statuses[ids[0]!]).toEqual(current);
  const signal = vi.mocked(listServerStatuses).mock.calls[0]![0];
  unmount();
  expect(signal.aborted).toBe(true);
});
