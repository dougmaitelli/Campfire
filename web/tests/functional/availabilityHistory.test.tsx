import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { PublicServerStatus } from "../../src/components/servers/PublicServerStatus";
import type {
  AvailabilityHistory,
  PublicStatusEntry,
} from "../../src/api/statusPages";
import { PublicServerState } from "../../src/contracts/domains";

const history: AvailabilityHistory = {
  from: 1800000000 - 86400,
  to: 1800000000,
  interval: 300,
  points: Array.from({ length: 288 }, (_, i) => ({
    online_seconds: i < 144 ? 300 : 0,
    offline_seconds: i >= 144 && i < 216 ? 300 : 0,
  })),
};
const entry: PublicStatusEntry = {
  id: "entry",
  name: "Game",
  state: PublicServerState.Online,
  observed_at: history.to,
  game_address: "game.example.test:25565",
  history,
};
afterEach(cleanup);

test("history shows bounded green/red/yellow coverage behind readable current status", () => {
  const { container } = render(
    <PublicServerStatus entry={entry} now={history.to + 1} failed={false} />,
  );
  const graph = screen.getByRole("img", {
    name: /50% online, 25% offline, 25% unknown/,
  });
  expect(graph.getAttribute("viewBox")).toBe("0 0 148.25 900");
  const colorPaths = graph.querySelectorAll("g > path");
  expect(colorPaths).toHaveLength(3);
  expect(graph.querySelector(".history-online")?.getAttribute("d")).toMatch(
    /^M0,0/,
  );
  expect(graph.querySelector(".history-offline")?.getAttribute("d")).toMatch(
    /^M74\./,
  );
  expect(graph.querySelector(".history-unknown")?.getAttribute("d")).toMatch(
    /^M111\./,
  );
  const bars = Array.from(colorPaths).flatMap((path) => {
    const d = path.getAttribute("d") ?? "";
    expect(d).not.toContain("q"); // Color changes are never rounded.
    return [...d.matchAll(/M([\d.]+),[\d.]+h([\d.]+)/g)];
  });
  expect(bars).toHaveLength(96);
  expect(bars.every((bar) => Number(bar[2]) === 1.0)).toBe(true);
  const positions = bars.map((bar) => Number(bar[1])).sort((a, b) => a - b);
  expect(positions[0]).toBe(0);
  expect(positions[95]).toBeCloseTo(147.25);
  expect(positions[1]).toBeCloseTo(1.55);
  const outline = graph.querySelector("clipPath path")?.getAttribute("d");
  expect(outline).toMatch(/a0\.5,[\d.]+ 0 0 1 1,0V900Z/); // Rounded tops, square bottoms.
  expect(screen.getByText("Online")).toBeTruthy();
  expect(screen.getByRole("heading", { name: "Game" })).toBeTruthy();
  expect(screen.getByRole("button", { name: /Copy/ })).toBeTruthy();
  expect(container.querySelector("li")?.firstElementChild).toBe(graph);
});

test("refresh failure expires current status without recoloring or advancing historical observations", () => {
  const { rerender } = render(
    <PublicServerStatus entry={entry} now={history.to + 1} failed={false} />,
  );
  const before = screen.getByRole("img").outerHTML;
  rerender(<PublicServerStatus entry={entry} now={history.to + 60} failed />);
  expect(screen.getByText("Unknown")).toBeTruthy();
  expect(screen.getByRole("img").outerHTML).toBe(before);
});

test("unobserved history is unknown, not downtime", () => {
  render(
    <PublicServerStatus
      entry={{
        ...entry,
        history: {
          ...history,
          points: history.points.map(() => ({
            online_seconds: 0,
            offline_seconds: 0,
          })),
        },
      }}
      now={history.to}
      failed={false}
    />,
  );
  const graph = screen.getByRole("img", {
    name: /0% online, 0% offline, 100% unknown/,
  });
  expect(graph.querySelector(".history-offline")?.getAttribute("d")).toBe("");
  expect(graph.querySelector(".history-online")?.getAttribute("d")).toBe("");
});
