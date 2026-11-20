import { afterEach, expect, test, vi } from "vitest";
import { isEmber } from "../../src/api/embers";
import { listAudit } from "../../src/api/activity";
import {
  activityFilterFromQuery,
  readActivity,
} from "../../src/api/serverActivity";
import { completeEmber } from "../fixtures/ember";
import { pageFixture } from "../pageFixture";

afterEach(() => vi.restoreAllMocks());

test.each(["succeeded", "failed", "interrupted", "success", "failure"])(
  "SFTP audit records use canonical outcomes: %s",
  async (outcome) => {
    const body = pageFixture([
      {
        id: "sftp-audit",
        user_id: null,
        username: "operator",
        api_token_id: 1,
        action: "server.files.sftp.download",
        target: "server:game",
        outcome,
        status: 200,
        created_at: 1,
      },
    ]);
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(body));
    const response = listAudit(new AbortController().signal);
    if (["success", "failure"].includes(outcome))
      await expect(response).rejects.toThrow();
    else await expect(response).resolves.toEqual(body);
  },
);

// Boundary fixtures deliberately use raw JSON strings, not enum members.
test.each(["game", "operation", "job", "alert", "notification"])(
  "activity accepts the existing %s wire kind and preserves open-ended states",
  async (kind) => {
    const body = pageFixture([
      {
        key: "entry",
        kind,
        title: "Event",
        state: "provider-specific-state",
        source_available: true,
        created_at: 1,
        notification_count: 0,
      },
    ]);
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(body));
    await expect(
      readActivity("server", {}, new AbortController().signal),
    ).resolves.toEqual(body);
  },
);

test.each(["unknown", ["game"], null, 0, {}].map((kind) => ({ kind })))(
  "activity rejects malformed wire kind %# without string coercion",
  async ({ kind }) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json(
        pageFixture([
          {
            key: "entry",
            kind,
            title: "Event",
            state: "pending",
            source_available: true,
            created_at: 1,
            notification_count: 0,
          },
        ]),
      ),
    );
    await expect(
      readActivity("server", {}, new AbortController().signal),
    ).rejects.toThrow();
  },
);

test.each(["game", "operations", "alert", "unknown", "operation"])(
  "activity filter validates URL kind %s",
  (kind) => {
    const filter = activityFilterFromQuery(new URLSearchParams({ kind }));
    expect(filter.kind).toBe(
      ["game", "operations", "alert"].includes(kind) ? kind : "",
    );
  },
);

test.each(
  ["sh", "bash", "ash", "unknown", ["sh"], null, 0].map((startup_shell) => ({
    startup_shell,
  })),
)(
  "Ember shell validation preserves supported wire values %#",
  ({ startup_shell }) => {
    expect(isEmber({ ...completeEmber(), startup_shell })).toBe(
      typeof startup_shell === "string" &&
        ["sh", "bash", "ash"].includes(startup_shell),
    );
  },
);
