import { expect, test, vi } from "vitest";
import { manageFiles } from "../../src/api/files";
import {
  compressionStates,
  isCompressionTerminal,
} from "../../src/api/fileTypes";

const request = {
  operation: "compress.status" as const,
  path: "/source",
  destination: "/archive.tar.gz",
  compression: { id: "operation" },
};
test.each([
  ["running", false],
  ["cancelling", false],
  ["completed", true],
  ["cancelled", true],
  ["failed", true],
] as const)("classifies %s terminal state as %s", (state, terminal) => {
  expect(isCompressionTerminal(state)).toBe(terminal);
});
const base = { server_id: "game", path: "/archive.tar.gz" };
test.each([
  undefined,
  null,
  {},
  { id: "other", state: "completed" },
  { id: "operation", state: "unknown" },
  { id: "operation", state: "running", work_progress: {} },
  { id: "operation", state: "failed", message: 7 },
])("rejects malformed compression snapshots %#", async (compression) => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    Response.json({
      ...base,
      ...(compression === undefined ? {} : { compression }),
    }),
  );
  await expect(manageFiles("game", request)).rejects.toThrow(
    "Unexpected file response",
  );
});
test.each(compressionStates)("accepts compression state %s", async (state) => {
  const body = { ...base, compression: { id: "operation", state } };
  vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(body));
  await expect(manageFiles("game", request)).resolves.toEqual(body);
});
