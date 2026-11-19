import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { manageFiles } from "../../src/api/files";
import { compressArchive } from "../../src/components/files/compressArchive";
import { FileOperation } from "../../src/contracts/domains";
vi.mock("../../src/api/files", () => ({ manageFiles: vi.fn() }));
beforeEach(() => {
  vi.resetAllMocks();
  vi.useFakeTimers();
});
afterEach(() => vi.useRealTimers());
const args = () => ({
  id: "server",
  path: "/source",
  destination: "/archive.tar.gz",
  signal: new AbortController().signal,
  onProgress: vi.fn(),
});

test("deadline failure is surfaced instead of resolving as normal cancellation", async () => {
  const message =
    "Compression exceeded its time limit. Try compressing fewer files.";
  vi.mocked(manageFiles).mockImplementation(async (id, input) => ({
    server_id: id,
    path: input.destination!,
    compression: {
      id: input.compression!.id,
      state:
        input.operation === FileOperation.CompressStart ? "running" : "failed",
      message,
    },
  }));
  const options = args();
  const result = expect(compressArchive(options)).rejects.toThrow(message);
  await vi.advanceTimersByTimeAsync(1000);
  await result;
  expect(options.onProgress).toHaveBeenLastCalledWith(undefined);
  expect(
    vi.mocked(manageFiles).mock.calls.map((call) => call[1].operation),
  ).toEqual([FileOperation.CompressStart, FileOperation.CompressStatus]);
});

test("waits for publication confirmation even after all measured bytes", async () => {
  let requests = 0;
  vi.mocked(manageFiles).mockImplementation(async (id, input) => ({
    server_id: id,
    path: input.destination!,
    compression: {
      id: input.compression!.id,
      state: ++requests === 1 ? "running" : "completed",
      work_progress: {
        phase: "archiving",
        input_bytes: 7,
        input_total: 7,
        output_bytes: 20,
        entries: 1,
        updated_at: new Date().toISOString(),
      },
    },
  }));
  let done = false;
  const promise = compressArchive(args()).then(() => {
    done = true;
  });
  await vi.advanceTimersByTimeAsync(0);
  expect(done).toBe(false);
  await vi.advanceTimersByTimeAsync(1000);
  await promise;
  expect(manageFiles).toHaveBeenCalledTimes(2);
  expect(vi.mocked(manageFiles).mock.calls[1]![1].operation).toBe(
    FileOperation.CompressStatus,
  );
});
test("lost start acknowledgement reconciles the same operation without restarting", async () => {
  vi.mocked(manageFiles).mockImplementation(async (id, input) => {
    if (input.operation === FileOperation.CompressStart)
      throw new TypeError("Failed to fetch");
    return {
      server_id: id,
      path: input.destination!,
      compression: { id: input.compression!.id, state: "completed" },
    };
  });
  await compressArchive(args());
  const calls = vi.mocked(manageFiles).mock.calls;
  expect(calls.map((call) => call[1].operation)).toEqual([
    FileOperation.CompressStart,
    FileOperation.CompressStatus,
  ]);
  expect(calls[0]![1].compression!.id).toBe(calls[1]![1].compression!.id);
});
test("lost cancellation acknowledgement reports uncertainty, not success", async () => {
  vi.mocked(manageFiles).mockRejectedValue(new TypeError("Failed to fetch"));
  await expect(compressArchive(args())).rejects.toThrow(/unconfirmed/i);
  expect(
    vi.mocked(manageFiles).mock.calls.map((call) => call[1].operation),
  ).toEqual([
    FileOperation.CompressStart,
    FileOperation.CompressStatus,
    FileOperation.CompressCancel,
  ]);
});

test.each(["cancelled", "completed"] as const)(
  "cancellation reconciles a %s terminal acknowledgement",
  async (state) => {
    const controller = new AbortController();
    vi.mocked(manageFiles).mockImplementation(async (id, input) => {
      if (input.operation === FileOperation.CompressStart) {
        controller.abort();
        throw new DOMException("Aborted", "AbortError");
      }
      return {
        server_id: id,
        path: input.destination!,
        compression: { id: input.compression!.id, state },
      };
    });
    await expect(
      compressArchive({ ...args(), signal: controller.signal }),
    ).resolves.toBe(state === "completed");
    expect(vi.mocked(manageFiles).mock.calls[1]![1].operation).toBe(
      FileOperation.CompressCancel,
    );
  },
);
