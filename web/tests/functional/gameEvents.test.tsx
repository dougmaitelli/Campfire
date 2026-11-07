import { expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { ServerGameEvents } from "../../src/components/servers/ServerGameEvents";

// The shared dropdown's browser interactions are covered by Playwright.
vi.mock("../../src/components/forms/Select", () => ({
  Select: ({
    onValueChange,
    ...props
  }: ComponentProps<"select"> & {
    onValueChange: (value: string) => void;
  }) => (
    <select
      {...props}
      onChange={(event) => onValueChange(event.target.value)}
    />
  ),
}));

const filterOptions = {
  types: ["game.announcement", "player.joined"],
  fields: { message: ["<script>alert(1)</script>"], player: ["Alex"] },
};

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
    if (url.pathname.endsWith("/filters")) return Response.json(filterOptions);
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
  await user.selectOptions(
    screen.getByLabelText("Event type"),
    "player.joined",
  );
  await user.click(screen.getByRole("button", { name: "Apply filters" }));
  await waitFor(() =>
    expect(queries.at(-1)?.searchParams.get("type")).toBe("player.joined"),
  );
  expect(queries.at(-1)?.searchParams.has("before")).toBe(false);
});

test("game activity reports request failures and retries", async () => {
  let failed = false;
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async (input) => {
      if (String(input).endsWith("/filters"))
        return Response.json(filterOptions);
      if (!failed) {
        failed = true;
        throw new TypeError("Connection unavailable");
      }
      return Response.json({ items: [event] });
    });
  render(<ServerGameEvents serverID="game" />);
  expect((await screen.findByRole("alert")).textContent).toContain(
    "Connection unavailable",
  );
  await userEvent.setup().click(screen.getByRole("button", { name: "Retry" }));
  await screen.findByText("A new adventure");
  expect(fetch).toHaveBeenCalledTimes(3);
});

test("game activity renders protected image and clip attachments without autoplay", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) =>
    String(input).endsWith("/filters")
      ? Response.json(filterOptions)
      : Response.json({
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
  expect(screen.getByText("1 KiB")).toBeDefined();
  expect(screen.getByText("4 KiB")).toBeDefined();
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
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) =>
    String(input).endsWith("/filters")
      ? Response.json(filterOptions)
      : Response.json({
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

test("filter choices retry independently without hiding activity", async () => {
  let failed = false;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    if (!String(input).endsWith("/filters"))
      return Response.json({ items: [event] });
    if (!failed) {
      failed = true;
      throw new TypeError("Choices unavailable");
    }
    return Response.json(filterOptions);
  });
  render(<ServerGameEvents serverID="game" />);
  await screen.findByText("A new adventure");
  expect((await screen.findByRole("alert")).textContent).toContain(
    "Choices unavailable",
  );
  expect(
    (screen.getByLabelText("Event type") as HTMLSelectElement).disabled,
  ).toBe(true);
  const user = userEvent.setup();
  await user.click(
    screen.getByRole("button", { name: "Retry filter options" }),
  );
  await waitFor(() =>
    expect(
      (screen.getByLabelText("Event type") as HTMLSelectElement).disabled,
    ).toBe(false),
  );
  await user.click(screen.getByRole("button", { name: "Add field filter" }));
  await user.selectOptions(screen.getByLabelText("Event field"), "player");
  await user.selectOptions(screen.getByLabelText("Field value"), "Alex");
  await user.selectOptions(screen.getByLabelText("Event field"), "message");
  expect(
    (screen.getByLabelText("Field value") as HTMLSelectElement).value,
  ).toBe("");
});
