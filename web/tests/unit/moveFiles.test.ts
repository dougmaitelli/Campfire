import { beforeEach, expect, test, vi } from "vitest";
import { APIError } from "../../src/api/client";
import { manageFiles } from "../../src/api/files";
import { FileKind, FileOperation } from "../../src/contracts/domains";
import { canMoveItems, moveFiles } from "../../src/components/files/moveFiles";

vi.mock("../../src/api/files", () => ({ manageFiles: vi.fn() }));
const moved = vi.fn();
const options = {
  serverID: "server",
  directory: "/source/",
  names: ["one", "two"],
  destination: "/destination/",
  onMoved: moved,
};
beforeEach(() => {
  moved.mockReset();
  vi.mocked(manageFiles)
    .mockReset()
    .mockImplementation(async (_, request) => ({
      server_id: "server",
      path:
        request.operation === FileOperation.List
          ? "/destination/"
          : (request.destination ?? request.path),
      entries: [],
    }));
});

test("move target rules reject empty selection, current folder, self and descendants but not prefix siblings", () => {
  expect(canMoveItems("/", [], "/dest/")).toBe(false);
  expect(canMoveItems("/", ["world"], "/")).toBe(false);
  expect(canMoveItems("/", ["world"], "/world/")).toBe(false);
  expect(canMoveItems("/", ["world"], "/world/region/")).toBe(false);
  expect(canMoveItems("/", ["world"], "/world-backup/")).toBe(true);
});

test("moves use canonical destination paths and checkpoint each confirmed item", async () => {
  await moveFiles(options);
  expect(manageFiles).toHaveBeenNthCalledWith(1, "server", {
    operation: FileOperation.List,
    path: "/destination/",
  });
  expect(manageFiles).toHaveBeenNthCalledWith(2, "server", {
    operation: FileOperation.Rename,
    path: "/source/one",
    destination: "/destination/one",
  });
  expect(manageFiles).toHaveBeenNthCalledWith(3, "server", {
    operation: FileOperation.Rename,
    path: "/source/two",
    destination: "/destination/two",
  });
  expect(moved.mock.calls).toEqual([["one"], ["two"]]);
});

test.each(["current", "descendant", "collision"])(
  "%s preflight rejects before any rename",
  async (kind) => {
    vi.mocked(manageFiles).mockResolvedValueOnce({
      server_id: "server",
      path:
        kind === "current"
          ? "/source/"
          : kind === "descendant"
            ? "/source/one/inside/"
            : "/destination/",
      entries:
        kind === "collision"
          ? [
              {
                name: "two",
                kind: FileKind.Directory,
                size: 0,
                modified_at: "2026-11-01",
              },
            ]
          : [],
    });
    await expect(moveFiles(options)).rejects.toThrow();
    expect(manageFiles).toHaveBeenCalledTimes(1);
    expect(moved).not.toHaveBeenCalled();
  },
);

test.each([
  new APIError("A new entry appeared.", 409),
  new Error("Connection lost"),
])(
  "stops after partial completion and distinguishes lost confirmation: %s",
  async (cause) => {
    vi.mocked(manageFiles)
      .mockResolvedValueOnce({
        server_id: "server",
        path: "/destination/",
        entries: [],
      })
      .mockResolvedValueOnce({ server_id: "server", path: "/destination/one" })
      .mockRejectedValueOnce(cause);
    await expect(
      moveFiles({ ...options, names: ["one", "two", "three"] }),
    ).rejects.toThrow(/1 of 3 moves confirmed/);
    expect(moved.mock.calls).toEqual([["one"]]);
    expect(manageFiles).toHaveBeenCalledTimes(3);
  },
);

test("lost confirmation explicitly asks for inspection rather than promising a failed move", async () => {
  vi.mocked(manageFiles)
    .mockResolvedValueOnce({
      server_id: "server",
      path: "/destination/",
      entries: [],
    })
    .mockRejectedValueOnce(new Error("Timed out"));
  await expect(moveFiles(options)).rejects.toThrow(
    /could not be confirmed.*Check both folders before retrying/,
  );
});
