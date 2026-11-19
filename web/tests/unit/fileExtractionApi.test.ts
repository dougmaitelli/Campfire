import {
  ExtractionState,
  ExtractionOutcomeStatus,
  FileOperation,
  FileKind,
} from "../../src/contracts/domains";
import { expect, test, vi } from "vitest";
import { manageFiles, type ExtractionStatus } from "../../src/api/files";

// Browser-only statuses must not become valid server extraction outcomes.
// @ts-expect-error Uploads are a distinct publication protocol.
const uploadOutcome: ExtractionOutcomeStatus = "uploaded";
// @ts-expect-error Unconfirmed is derived in the UI, never sent by Spark.
const uncertainOutcome: ExtractionOutcomeStatus = "unconfirmed";

const base = { server_id: "game", path: "/existing/" };
const operation: ExtractionStatus = {
  id: "operation",
  version: 1,
  state: ExtractionState.Completed,
  expires_at: "2026-11-17T12:00:00Z",
  pending_conflicts: 0,
  remaining_entries: 0,
  outcomes: [],
};
const request = {
  operation: FileOperation.ExtractStatus,
  path: "/input.zip",
  destination: "/existing",
  extraction: { id: "operation" },
};

test.each([
  undefined,
  null,
  {},
  { ...operation, id: "other-operation" },
  { ...operation, version: -1 },
  { ...operation, version: 0.5 },
  ...["unknown", ["completed"], 0, null, {}].map((state) => ({
    ...operation,
    state,
  })),
  { ...operation, expires_at: "invalid" },
  { ...operation, pending_conflicts: -1 },
  { ...operation, remaining_entries: -1 },
  { ...operation, pending_conflicts: 1, remaining_entries: 0 },
  { ...operation, outcomes: null },
  ...[uploadOutcome, uncertainOutcome].map((status) => ({
    ...operation,
    outcomes: [{ path: "a", destination: "existing/a", status }],
  })),
  ...["unknown", ["extracted"], 0, null, {}].map((status) => ({
    ...operation,
    outcomes: [{ path: "a", destination: "existing/a", status }],
  })),
  {
    ...operation,
    outcomes: [{ path: 3, destination: "existing/a", status: "extracted" }],
  },
  {
    ...operation,
    outcomes: [
      { path: "a", destination: "existing/a", status: "failed", message: null },
    ],
  },
  {
    ...operation,
    pending_conflicts: 1,
    remaining_entries: 1,
    conflict: {
      id: "conflict",
      path: "a",
      destination: "existing/a",
      kind: "unsupported",
      size: 3,
    },
  },
  {
    ...operation,
    pending_conflicts: 1,
    remaining_entries: 1,
    conflict: {
      id: "conflict",
      path: "a",
      destination: "existing/a",
      kind: "file",
      size: -1,
    },
  },
  {
    ...operation,
    pending_conflicts: 1,
    remaining_entries: 1,
    conflict: {
      id: "conflict",
      path: "a",
      destination: "existing/a",
      kind: "file",
      size: 1,
      existing: { kind: "file" },
    },
  },
])(
  "rejects malformed extraction operation snapshots %#",
  async (extraction) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        ...base,
        ...(extraction === undefined ? {} : { extraction }),
      }),
    );
    await expect(manageFiles("game", request)).rejects.toThrow(
      "Unexpected file response",
    );
  },
);

test.each(Object.values(ExtractionOutcomeStatus))(
  "accepts the declared Spark outcome %s",
  async (status) => {
    const body = {
      ...base,
      extraction: {
        ...operation,
        outcomes: [{ path: "a", destination: "existing/a", status }],
      },
    };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(body));
    await expect(manageFiles("game", request)).resolves.toEqual(body);
  },
);

test.each(Object.values(ExtractionState))(
  "accepts the declared extraction state %s",
  async (state) => {
    const body = { ...base, extraction: { ...operation, state } };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(body));
    await expect(manageFiles("game", request)).resolves.toEqual(body);
  },
);

test("accepts empty completion and live conflict snapshots", async () => {
  const snapshots: ExtractionStatus[] = [
    operation,
    {
      ...operation,
      state: ExtractionState.Waiting,
      pending_conflicts: 1,
      remaining_entries: 1,
      outcomes: [
        {
          path: "a",
          destination: "existing/a",
          status: ExtractionOutcomeStatus.Pending,
        },
      ],
      conflict: {
        id: "conflict",
        path: "a",
        destination: "existing/a",
        kind: FileKind.File,
        size: 3,
        changed: true,
        existing: {
          name: "a",
          kind: FileKind.File,
          size: 3,
          modified_at: "2026-11-01T00:00:00Z",
        },
      },
    },
  ];
  for (const extraction of snapshots) {
    const body = { ...base, extraction };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(body));
    await expect(manageFiles("game", request)).resolves.toEqual(body);
  }
});
