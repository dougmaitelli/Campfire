import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { PublicStatusPage } from "../../src/pages/PublicStatusPage";
import type { PublicStatusPage as Snapshot } from "../../src/api/statusPages";
import { PublicServerState } from "../../src/contracts/domains";

const serverNow = 1790000000;
const snapshot: Snapshot = {
  title: "Community",
  collected_at: serverNow,
  poll_seconds: 60,
  entries: [
    {
      id: "entry",
      name: "Game",
      state: PublicServerState.Online,
      observed_at: serverNow - 12,
      game_state: {
        updated_at: serverNow - 58,
        state: { players: 3, capacity: 20 },
      },
    },
  ],
};
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
function clock(offset: number) {
  vi.useFakeTimers({
    toFake: [
      "Date",
      "performance",
      "setInterval",
      "clearInterval",
      "setTimeout",
      "clearTimeout",
    ],
  });
  vi.setSystemTime((serverNow + offset) * 1000);
}

test.each([-120, 120])(
  "public freshness follows Campfire time with browser skew of %s seconds",
  async (offset) => {
    clock(offset);
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(snapshot));
    await act(async () => {
      render(<PublicStatusPage slug="community" />);
    });
    expect(screen.getByText("Online")).toBeTruthy();
    expect(screen.getByText("3 / 20")).toBeTruthy();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(screen.getByText("Online")).toBeTruthy();
    expect(screen.queryByText("3 / 20")).toBeNull();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(screen.getByText("Unknown")).toBeTruthy();
  },
);

test.each([-3600, 3600])(
  "wall-clock adjustment of %s seconds does not change the freshness deadline",
  async (jump) => {
    clock(0);
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(snapshot));
    await act(async () => {
      render(<PublicStatusPage slug="community" />);
    });
    await act(async () => {
      vi.setSystemTime((serverNow + jump) * 1000);
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(screen.getByText("Online")).toBeTruthy();
    expect(screen.getByText("3 / 20")).toBeTruthy();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(screen.getByText("Unknown")).toBeTruthy();
    expect(screen.queryByText("3 / 20")).toBeNull();
  },
);

test("a delayed response cannot renew the server observations", async () => {
  clock(120);
  let complete!: (response: Response) => void;
  vi.spyOn(globalThis, "fetch").mockImplementation(
    () =>
      new Promise<Response>((resolve) => {
        complete = resolve;
      }),
  );
  await act(async () => {
    render(<PublicStatusPage slug="community" />);
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(4000);
    complete(Response.json(snapshot));
  });
  expect(screen.getByText("Unknown")).toBeTruthy();
  expect(screen.queryByText("3 / 20")).toBeNull();
});
