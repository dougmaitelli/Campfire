import { expect, test, vi } from "vitest";
import { readPublicStatusPage, statusPageURL } from "../../src/api/statusPages";

const valid = {
  title: "Community",
  collected_at: 12,
  poll_seconds: 10,
  entries: [
    {
      id: "entry",
      name: "Game",
      state: "online",
      observed_at: 10,
    },
  ],
};
test.each(["online", "offline", "unknown"])(
  "public availability accepts %s",
  async (state) => {
    const value = { ...valid, entries: [{ ...valid.entries[0], state }] };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(value));
    await expect(
      readPublicStatusPage("community", new AbortController().signal),
    ).resolves.toEqual(value);
  },
);
test.each([1, 5, 10, 60])(
  "public polling accepts a bounded interval of %s seconds",
  async (seconds) => {
    const value = { ...valid, poll_seconds: seconds };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(value));
    await expect(
      readPublicStatusPage("community", new AbortController().signal),
    ).resolves.toEqual(value);
  },
);

test.each([
  { ...valid, collected_at: undefined },
  { ...valid, collected_at: -1 },
  { ...valid, collected_at: 1.5 },
  { ...valid, collected_at: "12" },
  { ...valid, collected_at: null },
  { ...valid, poll_seconds: 0 },
  { ...valid, poll_seconds: -1 },
  { ...valid, poll_seconds: 0.5 },
  { ...valid, poll_seconds: 61 },
  { ...valid, poll_seconds: "5" },
  { ...valid, poll_seconds: null },
  { ...valid, entries: [{ ...valid.entries[0], state: "running" }] },
  {
    ...valid,
    entries: [{ ...valid.entries[0], game_state: { updated_at: -1 } }],
  },
  {
    ...valid,
    entries: [
      {
        ...valid.entries[0],
        game_state: { updated_at: 1, state: { players: -1 } },
      },
    ],
  },
  { ...valid, entries: [{ ...valid.entries[0], art_url: 123 }] },
  { ...valid, entries: [{ ...valid.entries[0], game_address: [] }] },
  { ...valid, entries: [{ ...valid.entries[0], observed_at: -1 }] },
  { ...valid, entries: Array.from({ length: 51 }, () => valid.entries[0]) },
])(
  "public response rejects invalid availability metadata %#",
  async (value) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(value));
    await expect(
      readPublicStatusPage("community", new AbortController().signal),
    ).rejects.toThrow();
  },
);
test("public URLs remain rooted at the standalone route", () => {
  expect(statusPageURL("community")).toBe("/status/community");
});
