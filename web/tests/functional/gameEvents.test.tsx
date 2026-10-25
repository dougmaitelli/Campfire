import { expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ServerGameEvents } from "../../src/components/servers/ServerGameEvents";

const event = {
  id: 3,
  server_id: "game",
  spark_id: "node",
  event_id: "event-3",
  type: "game.announcement",
  title: "A new adventure",
  occurred_at: "2026-10-24T00:00:00Z",
  received_at: 1792800000,
  data: { message: "<script>alert(1)</script>" },
};

test("game activity paginates, filters, and renders event data as text", async () => {
  const queries: URL[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = new URL(String(input), "https://panel.test");
    queries.push(url);
    return Response.json(
      url.searchParams.has("before")
        ? { items: [] }
        : { items: [event], next_before: 3 },
    );
  });
  const user = userEvent.setup();
  const view = render(<ServerGameEvents serverID="game" />);
  await screen.findByText("A new adventure");
  expect(screen.getByText("<script>alert(1)</script>")).toBeDefined();
  expect(view.container.querySelector("script")).toBeNull();
  await user.click(screen.getByRole("button", { name: "Older events" }));
  await screen.findByRole("heading", { name: "No game events found" });
  expect(queries.at(-1)?.searchParams.get("before")).toBe("3");
  await user.click(screen.getByRole("button", { name: "Newest events" }));
  await screen.findByText("A new adventure");
  await user.type(screen.getByLabelText("Event type"), "player.joined");
  await user.click(screen.getByRole("button", { name: "Apply filters" }));
  await waitFor(() =>
    expect(queries.at(-1)?.searchParams.get("type")).toBe("player.joined"),
  );
  expect(queries.at(-1)?.searchParams.has("before")).toBe(false);
});

test("game activity reports request failures and retries", async () => {
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockRejectedValueOnce(new TypeError("Connection unavailable"))
    .mockResolvedValue(Response.json({ items: [event] }));
  render(<ServerGameEvents serverID="game" />);
  expect((await screen.findByRole("alert")).textContent).toContain(
    "Connection unavailable",
  );
  await userEvent.setup().click(screen.getByRole("button", { name: "Retry" }));
  await screen.findByText("A new adventure");
  expect(fetch).toHaveBeenCalledTimes(2);
});

test("game activity renders protected image and clip attachments without autoplay", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    Response.json({
      items: [
        {
          ...event,
          attachments: [
            {
              id: "shot",
              name: "world.png",
              content_type: "image/png",
              size: 1024,
            },
            {
              id: "clip",
              name: "moment.webm",
              content_type: "video/webm",
              size: 4096,
            },
          ],
        },
      ],
    }),
  );
  const view = render(<ServerGameEvents serverID="game" />);
  const image = await screen.findByRole("img", { name: "world.png" });
  expect(image.getAttribute("src")).toBe(
    "/api/v1/servers/game/events/3/media/shot",
  );
  expect(
    screen
      .getByRole("link", { name: "Download world.png" })
      .getAttribute("href"),
  ).toBe("/api/v1/servers/game/events/3/media/shot?download=1");
  const video = view.container.querySelector("video");
  expect(video?.hasAttribute("controls")).toBe(true);
  expect(video?.hasAttribute("autoplay")).toBe(false);
  expect(video?.getAttribute("preload")).toBe("none");
  expect(video?.querySelector("source")?.getAttribute("src")).toBe(
    "/api/v1/servers/game/events/3/media/clip",
  );
});

test("game activity rejects unsupported media and external attachment IDs", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    Response.json({
      items: [
        {
          ...event,
          attachments: [
            {
              id: "https://evil.test/shot",
              name: "world.svg",
              content_type: "image/svg+xml",
              size: 1024,
            },
          ],
        },
      ],
    }),
  );
  const view = render(<ServerGameEvents serverID="game" />);
  expect((await screen.findByRole("alert")).textContent).toContain(
    "Unexpected game event response",
  );
  expect(view.container.querySelector("img,video")).toBeNull();
});
