import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { App } from "../../src/App";
import { PublicStatusPage } from "../../src/pages/PublicStatusPage";
import type { PublicStatusPage as Snapshot } from "../../src/api/statusPages";
import { PublicServerState } from "../../src/contracts/domains";

const page: Snapshot = {
  title: "Community status",
  collected_at: 1790000012,
  poll_seconds: 10,
  entries: [
    {
      id: "public-entry",
      name: "Community game",
      state: PublicServerState.Online,
      observed_at: 1790000000,
    },
  ],
};
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  window.history.replaceState(null, "", "/");
});

test("public route mounts no authenticated shell and sends no credentials", async () => {
  window.history.replaceState(null, "", "/status/community");
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(Response.json(page));
  render(<App />);
  await screen.findByRole("heading", { name: "Community status" });
  expect(screen.getByText("Campfire")).toBeTruthy();
  expect(
    screen.getByRole("heading", { name: "Community game", level: 3 }),
  ).toBeTruthy();
  expect(screen.queryByRole("navigation")).toBeNull();
  expect(screen.queryByRole("link")).toBeNull();
  expect(screen.queryByText("Notifications")).toBeNull();
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch).toHaveBeenCalledWith(
    "/api/v1/public/status-pages/community",
    expect.objectContaining({
      credentials: "omit",
      cache: "no-store",
      redirect: "error",
    }),
  );
});

test("availability expires locally, becomes unknown on a lost refresh, and recovers", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(1790000012000);
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValueOnce(Response.json(page))
    .mockRejectedValueOnce(new TypeError("offline"))
    .mockImplementation(async () =>
      Response.json({
        ...page,
        collected_at: Math.floor(Date.now() / 1000),
        entries: page.entries.map((entry) => ({
          ...entry,
          state: PublicServerState.Offline,
          observed_at: Math.floor(Date.now() / 1000) - 1,
        })),
      }),
    );
  await act(async () => {
    render(<PublicStatusPage slug="community" />);
  });
  expect(screen.getByText("Online")).toBeTruthy();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(4000);
  });
  expect(screen.getByText("Unknown")).toBeTruthy();
  expect(fetch).toHaveBeenCalledTimes(1);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(6000);
  });
  expect(screen.getByText("Unknown")).toBeTruthy();
  expect(screen.getByText("Community game")).toBeTruthy();
  expect(screen.getByText(/Unable to refresh status/)).toBeTruthy();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(10000);
  });
  expect(screen.getByText("Offline")).toBeTruthy();
  expect(screen.queryByText(/Unable to refresh status/)).toBeNull();
});

test("request time counts against freshness and public names remain plain text", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(1790000012000);
  let complete: (response: Response) => void = () => {
    throw new Error("request not started");
  };
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
    complete(
      Response.json({
        ...page,
        entries: page.entries.map((entry) => ({
          ...entry,
          name: "<img src=x onerror=alert(1)>",
        })),
      }),
    );
  });
  expect(screen.getByText("Unknown")).toBeTruthy();
  expect(
    screen.getByRole("heading", { name: "<img src=x onerror=alert(1)>" }),
  ).toBeTruthy();
  expect(document.querySelector(".public-status img")).toBeNull();
});

test("successful polls use the returned interval while failures use the retry interval", async () => {
  vi.useFakeTimers();
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValueOnce(Response.json({ ...page, poll_seconds: 5 }))
    .mockRejectedValueOnce(new TypeError("offline"))
    .mockImplementation(async () =>
      Response.json({ ...page, poll_seconds: 2 }),
    );
  await act(async () => {
    render(<PublicStatusPage slug="community" />);
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(4999);
  });
  expect(fetch).toHaveBeenCalledTimes(1);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(fetch).toHaveBeenCalledTimes(2);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(9999);
  });
  expect(fetch).toHaveBeenCalledTimes(2);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(fetch).toHaveBeenCalledTimes(3);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1999);
  });
  expect(fetch).toHaveBeenCalledTimes(3);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(fetch).toHaveBeenCalledTimes(4);
});

test("a deleted public page clears old names and stops polling", async () => {
  vi.useFakeTimers();
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValueOnce(Response.json(page))
    .mockResolvedValue(
      Response.json({ error: "status page not found" }, { status: 404 }),
    );
  await act(async () => {
    render(<PublicStatusPage slug="community" />);
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(10000);
  });
  expect(
    screen.getByRole("heading", { name: "Status page not found" }),
  ).toBeTruthy();
  expect(screen.queryByText("Community game")).toBeNull();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(20000);
  });
  expect(fetch).toHaveBeenCalledTimes(2);
});

test("malformed public paths never mount login or make API requests", async () => {
  window.history.replaceState(null, "", "/status/INVALID/path");
  const fetch = vi.spyOn(globalThis, "fetch");
  render(<App />);
  await screen.findByRole("heading", { name: "Status page not found" });
  expect(screen.queryByRole("link")).toBeNull();
  expect(fetch).not.toHaveBeenCalled();
});
