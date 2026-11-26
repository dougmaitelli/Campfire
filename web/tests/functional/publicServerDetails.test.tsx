import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { PublicStatusPage } from "../../src/pages/PublicStatusPage";
import { PublicServerStatus } from "../../src/components/servers/PublicServerStatus";
import { PublicServerState } from "../../src/contracts/domains";
import type { PublicStatusEntry } from "../../src/api/statusPages";

const now = 1790000000;
const entry: PublicStatusEntry = {
  id: "opaque",
  name: "Community Minecraft",
  state: PublicServerState.Online,
  observed_at: now,
  art_url: "/game-art/mock-block.svg",
  game_address: "play.example.test:32100",
  game_state: {
    updated_at: now - 58,
    state: {
      players: 3,
      capacity: 20,
      fields: {
        map: "Community world",
        player_names: ["Alex", "Sam"],
        pvp: false,
        queue: 0,
        empty: [],
        text: "<img src=x onerror=alert(1)>",
      },
      field_labels: { map: "World name", player_names: "Online players" },
    },
  },
};
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

test("public cards reuse game artwork, labels, lists and formatting with an image fallback", () => {
  const { container, rerender } = render(
    <PublicServerStatus entry={entry} now={now} failed={false} />,
  );
  expect(screen.getByText("play.example.test:32100")).toBeTruthy();
  expect(screen.getByText("3 / 20")).toBeTruthy();
  expect(screen.getByText("World name")).toBeTruthy();
  expect(screen.getByText("Community world")).toBeTruthy();
  expect(screen.getByText("Alex, Sam")).toBeTruthy();
  expect(screen.getByText("No")).toBeTruthy();
  expect(screen.getByText("0")).toBeTruthy();
  expect(screen.getByText("None")).toBeTruthy();
  expect(screen.getByText("<img src=x onerror=alert(1)>")).toBeTruthy();
  const art = container.querySelector("img")!;
  expect(art.getAttribute("src")).toBe(entry.art_url);
  expect(art.getAttribute("referrerpolicy")).toBe("no-referrer");
  expect(container.querySelectorAll("img")).toHaveLength(1);
  fireEvent.error(art);
  expect(container.querySelector("img")).toBeNull();
  expect(
    container.querySelector(".public-status-server-icon svg"),
  ).not.toBeNull();
  rerender(
    <PublicServerStatus
      entry={{ ...entry, art_url: "/game-art/another.svg" }}
      now={now}
      failed={false}
    />,
  );
  expect(container.querySelector("img")?.getAttribute("src")).toBe(
    "/game-art/another.svg",
  );
});

test("game fields expire from the original receipt timestamp, not from page fetches", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(now * 1000);
  vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
    Response.json({
      title: "Community",
      poll_seconds: 1,
      collected_at: Math.floor(Date.now() / 1000),
      entries: [{ ...entry, observed_at: Math.floor(Date.now() / 1000) }],
    }),
  );
  await act(async () => {
    render(<PublicStatusPage slug="community" />);
  });
  expect(screen.getByText("Alex, Sam")).toBeTruthy();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(screen.getByText("Online")).toBeTruthy();
  expect(screen.queryByText("Alex, Sam")).toBeNull();
  expect(screen.getByText("play.example.test:32100")).toBeTruthy();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(3000);
  });
  expect(screen.queryByText("Alex, Sam")).toBeNull();
});

test("a failed refresh hides integration readings without discarding static public details", () => {
  render(<PublicServerStatus entry={entry} now={now} failed />);
  expect(screen.getByText("Unknown")).toBeTruthy();
  expect(screen.queryByText("Alex, Sam")).toBeNull();
  expect(screen.getByText("play.example.test:32100")).toBeTruthy();
});
