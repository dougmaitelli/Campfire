import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  expect,
  test,
  vi,
} from "vitest";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode, type ComponentProps } from "react";
import { ServerActivity } from "../../src/components/servers/ServerActivity";
import { GameEventDetails } from "../../src/components/activity/GameEventDetails";
import { ActivityDetailsDrawer } from "../../src/components/activity/ActivityDetailsDrawer";
import { ActivityFilters } from "../../src/components/activity/ActivityFilters";
import {
  activityFilterFromQuery,
  readActivityDetails,
  serverActivityURL,
  type ActivityFilter,
} from "../../src/api/serverActivity";
import type { Server } from "../../src/api/servers";
import type { AuthState } from "../../src/api/auth";
import { replaceCurrentRoute } from "../../src/routing/useCurrentPage";

const authentication = vi.hoisted(() => ({
  session: null as AuthState | null,
}));
vi.mock("../../src/hooks/useAuthentication", () => ({
  useAuthentication: () => ({ session: authentication.session }),
}));
beforeEach(() => {
  authentication.session = null;
});

vi.mock("../../src/components/forms/Select", () => ({
  Select: ({
    onValueChange,
    ...props
  }: ComponentProps<"select"> & { onValueChange: (value: string) => void }) => (
    <select
      {...props}
      onChange={(event) => onValueChange(event.target.value)}
    />
  ),
}));
const dialogMethods = Object.getOwnPropertyDescriptors(
  HTMLDialogElement.prototype,
);
beforeAll(() => {
  Object.defineProperties(HTMLDialogElement.prototype, {
    showModal: {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.setAttribute("open", "");
      },
    },
    close: {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.removeAttribute("open");
      },
    },
  });
});
afterAll(() => {
  for (const name of ["showModal", "close"]) {
    const original = dialogMethods[name];
    if (original)
      Object.defineProperty(HTMLDialogElement.prototype, name, original);
    else Reflect.deleteProperty(HTMLDialogElement.prototype, name);
  }
});
afterEach(() => vi.useRealTimers());
const server: Server = {
  id: "game",
  name: "Test server",
  description: "",
  spark_id: "node",
  ember_id: "ember",
  image: "game:latest",
  variables: {},
  cpu_limit: 1,
  memory_mb: 1024,
  disk_mb: 1024,
  ports: [],
  ember_hash: "hash",
  ember_update_available: false,
  owner_user_id: 1,
  permissions: ["server.view"],
  spark_name: "Node",
  ember_name: "Game",
  game_address: "",
  status: "running",
  created_at: "2026-10-25",
};
const choices = {
  game: {
    types: ["game.announcement", "player.joined"],
    fields: { message: ["<script>alert(1)</script>"], player: ["Alex"] },
    field_labels: {
      "game.announcement": { message: "Announcement text" },
      "player.joined": { player: "Player name" },
    },
  },
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
  field_labels: { message: "Announcement text" },
};
const entry = {
  key: "game_event:3",
  kind: "game",
  source_available: false,
  created_at: event.received_at,
  title: event.title,
  state: "",
  notification_count: 0,
};
const details = {
  ...entry,
  game_event: event,
  can_retry_operation: false,
};
const operation = {
  id: "old",
  schedule_id: "",
  retry_of: "",
  recurring: false,
  server_id: "game",
  server_name: "Test",
  user_id: 1,
  username: "admin",
  action: "restart",
  state: "failed",
  message: "Original attempt failed",
  created_at: 1700000000,
  started_at: 1700000000,
  finished_at: 1700000001,
};
function start(filter: ActivityFilter = {}) {
  replaceCurrentRoute(serverActivityURL("game", filter).slice(1));
  return render(<ServerActivity server={server} />);
}

test("Activity paginates, keeps game filters in its URL and renders event data as text", async () => {
  const queries: URL[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = new URL(String(input), "https://panel.test");
    if (url.pathname.endsWith("/filters")) return Response.json(choices);
    if (!url.pathname.endsWith("/activity")) return Response.json(details);
    queries.push(url);
    return Response.json(
      url.searchParams.has("before")
        ? { items: [], next: "" }
        : { items: [entry], next: "page-two" },
    );
  });
  const user = userEvent.setup();
  const view = start();
  await screen.findByRole("button", { name: event.title });
  await user.click(screen.getByRole("button", { name: event.title }));
  await screen.findByRole("dialog", { name: event.title });
  expect(screen.getByText("<script>alert(1)</script>")).toBeDefined();
  expect(screen.getByText("Announcement text")).toBeDefined();
  expect(view.container.querySelector("script")).toBeNull();
  await user.click(screen.getAllByRole("button", { name: "Close" }).at(-1)!);
  await user.click(screen.getByRole("button", { name: "Older activity" }));
  await screen.findByRole("heading", {
    name: "No activity matches these filters.",
  });
  expect(queries.at(-1)?.searchParams.get("before")).toBe("page-two");
  await user.click(screen.getByRole("button", { name: "Newest activity" }));
  await screen.findByRole("button", { name: event.title });
  await user.selectOptions(screen.getByLabelText("Activity type"), "game");
  await user.selectOptions(
    await screen.findByLabelText("Event type"),
    "player.joined",
  );
  await user.click(screen.getByRole("button", { name: "Add field filter" }));
  expect(
    screen.getByRole("option", { name: "Player name" }).getAttribute("value"),
  ).toBe("player");
  await user.selectOptions(screen.getByLabelText("Event field"), "player");
  await user.selectOptions(screen.getByLabelText("Field value"), "Alex");
  await user.click(screen.getByRole("button", { name: "Apply filters" }));
  await waitFor(() =>
    expect(queries.at(-1)?.searchParams.get("data.player")).toBe("Alex"),
  );
  expect(queries.at(-1)?.searchParams.get("type")).toBe("player.joined");
  expect(queries.at(-1)?.searchParams.has("before")).toBe(false);
});

test("Activity reports request failures and retries", async () => {
  let failed = false;
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async (input) => {
      if (String(input).includes("/filters?")) return Response.json(choices);
      if (!failed) {
        failed = true;
        throw new TypeError("Connection unavailable");
      }
      return Response.json({ items: [entry], next: "" });
    });
  start();
  expect((await screen.findByRole("alert")).textContent).toContain(
    "Connection unavailable",
  );
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Try again" }));
  await screen.findByRole("button", { name: event.title });
  expect(fetch).toHaveBeenCalledTimes(2);
});

test("game details render protected image and clip attachments without autoplay", () => {
  const view = render(
    <GameEventDetails
      event={{
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
      }}
    />,
  );
  expect(
    screen.getByRole("img", { name: "world.png" }).getAttribute("src"),
  ).toBe("/api/v1/servers/game/events/3/media/shot");
  expect(screen.getByText("1 KiB")).toBeDefined();
  expect(screen.getByText("4 KiB")).toBeDefined();
  expect(
    screen
      .getByRole("link", { name: "Download world.png" })
      .getAttribute("href"),
  ).toBe("/api/v1/servers/game/events/3/media/shot?download=1");
  const video = view.container.querySelector("video");
  expect(video?.hasAttribute("controls")).toBe(true);
  expect(video?.hasAttribute("autoplay")).toBe(false);
  expect(video?.getAttribute("preload")).toBe("metadata");
  expect(video?.querySelector("source")?.getAttribute("src")).toBe(
    "/api/v1/servers/game/events/3/media/clip",
  );
});

test("related notifications distinguish text, image and video messages", async () => {
  const messages = [
    {},
    { attachment_name: "world.png", attachment_content_type: "image/png" },
    { attachment_name: "moment.webm", attachment_content_type: "video/webm" },
  ].map((attachment, index) => ({
    ...entry,
    ...attachment,
    key: `integration_message:${index + 1}`,
    kind: "notification",
    state: "delivered",
    provider: "discord",
    destination: "400",
  }));
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) =>
    Response.json(
      String(input).endsWith("/notifications")
        ? { items: messages, next: "" }
        : { ...details, notification_count: 3 },
    ),
  );
  render(
    <ActivityDetailsDrawer
      server={server}
      entryKey={entry.key}
      onOpen={vi.fn()}
      onClose={vi.fn()}
      onChanged={async () => {}}
    />,
  );
  const table = await screen.findByRole("table", { name: "Notifications" });
  expect(within(table).getByText("Text message")).toBeDefined();
  expect(within(table).getByText("Image · world.png")).toBeDefined();
  expect(within(table).getByText("Video · moment.webm")).toBeDefined();
});

test.each([true, false])(
  "operation initiator resource link respects user-management access %s",
  async (allowed) => {
    authentication.session = {
      authenticated: true,
      setup_required: false,
      admin: allowed,
      permissions: allowed ? ["users.manage"] : [],
    };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        ...entry,
        key: "operation:old",
        kind: "operation",
        title: "restart",
        state: "failed",
        operation,
        can_retry_operation: false,
      }),
    );
    render(
      <ActivityDetailsDrawer
        server={server}
        entryKey="operation:old"
        onOpen={vi.fn()}
        onClose={vi.fn()}
        onChanged={async () => {}}
      />,
    );
    await screen.findByText("admin");
    const link = screen.queryByRole("link", { name: "admin" });
    if (allowed) expect(link?.getAttribute("href")).toBe("#users/1");
    else expect(link).toBeNull();
  },
);

test("monitoring alerts are source activity with related notification delivery", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) =>
    Response.json(
      String(input).endsWith("/notifications")
        ? {
            items: [
              {
                ...entry,
                key: "notification:1",
                kind: "notification",
                title: "Test server — CPU alert",
                state: "created",
                source_key: "monitoring_alert:cpu",
                source_available: true,
              },
            ],
            next: "",
          }
        : {
            ...entry,
            key: "monitoring_alert:cpu",
            kind: "alert",
            state: "raised",
            title: "Test server — CPU alert",
            notification_count: 1,
            monitoring_alert: {
              id: "cpu",
              target_type: "server",
              target_id: server.id,
              code: "CPU",
              kind: "alert",
              severity: "warning",
              title: "Test server — CPU alert",
              body: "CPU usage exceeded the configured threshold.",
              created_at: entry.created_at,
            },
            can_retry_operation: false,
          },
    ),
  );
  render(
    <ActivityDetailsDrawer
      server={server}
      entryKey="monitoring_alert:cpu"
      onOpen={vi.fn()}
      onClose={vi.fn()}
      onChanged={async () => {}}
    />,
  );
  expect(
    await screen.findByText("CPU usage exceeded the configured threshold."),
  ).toBeDefined();
  expect(screen.getByText("Raised")).toBeDefined();
  expect(
    await screen.findByRole("table", { name: "Notifications" }),
  ).toBeDefined();
  expect(screen.queryByRole("button", { name: "Dismiss alert" })).toBeNull();
});

test.each([
  {
    ...details,
    game_event: {
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
  },
  { ...details, game_event: { ...event, server_id: "other" } },
  {
    ...entry,
    can_retry_operation: false,
    notification: {
      message_id: 1,
      name: "Discord",
      body: "",
      attempts: 0,
      next_attempt_at: 0,
      delivered_at: 0,
      error: "",
      can_retry: false,
      external_url: "javascript:alert(1)",
    },
  },
])(
  "Activity rejects invalid media, foreign server data and unsafe external links",
  async (value) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(value));
    await expect(
      readActivityDetails("game", entry.key, new AbortController().signal),
    ).rejects.toThrow("Unexpected activity response");
  },
);

test("filter choices retry independently while Activity remains available", async () => {
  let failed = false;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    if (!String(input).includes("/filters?"))
      return Response.json({ items: [entry], next: "" });
    if (!failed) {
      failed = true;
      throw new TypeError("Choices unavailable");
    }
    return Response.json(choices);
  });
  start({ kind: "game" });
  await screen.findByRole("button", { name: event.title });
  await screen.findByText("Choices unavailable");
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Try again" }));
  await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
  await user.selectOptions(screen.getByLabelText("Activity type"), "game");
  await user.click(
    await screen.findByRole("button", { name: "Add field filter" }),
  );
  await user.selectOptions(screen.getByLabelText("Event field"), "player");
  await user.selectOptions(screen.getByLabelText("Field value"), "Alex");
  await user.selectOptions(screen.getByLabelText("Event field"), "message");
  expect(
    (screen.getByLabelText("Field value") as HTMLSelectElement).value,
  ).toBe("");
});

test.each(["9007199254740991", "253402300800", "-1", "Infinity", "NaN", "1.5"])(
  "invalid Activity date %s is ignored without breaking the filters",
  (value) => {
    const filter = activityFilterFromQuery(
      new URLSearchParams({ from: value, to: value }),
    );
    expect(filter.from).toBeUndefined();
    expect(filter.to).toBeUndefined();
    render(
      <ActivityFilters filter={filter} choices={undefined} onApply={vi.fn()} />,
    );
    expect(
      (screen.getByLabelText("Recorded after") as HTMLInputElement).value,
    ).toBe("");
  },
);

test("valid Activity date bounds survive URL parsing", () => {
  const filter = activityFilterFromQuery(
    new URLSearchParams("from=1&to=253402300799"),
  );
  expect(filter.from).toBe(1);
  expect(filter.to).toBe(253402300799);
});

test("selected event fields and values remain visible after discovery expires", async () => {
  const apply = vi.fn();
  const filter = {
    kind: "game" as const,
    type: "player.joined",
    fields: { player: "Retired player" },
  };
  const view = render(
    <ActivityFilters filter={filter} choices={choices} onApply={apply} />,
  );
  expect(
    (screen.getByLabelText("Field value") as HTMLSelectElement).value,
  ).toBe("Retired player");
  expect(screen.getByRole("option", { name: "Player name" })).toBeDefined();
  view.rerender(
    <ActivityFilters
      filter={filter}
      choices={{ ...choices, game: { types: [], fields: {} } }}
      onApply={apply}
    />,
  );
  expect(
    (screen.getByLabelText("Event field") as HTMLSelectElement).value,
  ).toBe("player");
  expect(
    (screen.getByLabelText("Field value") as HTMLSelectElement).value,
  ).toBe("Retired player");
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Apply filters" }));
  expect(apply).toHaveBeenCalledWith(
    expect.objectContaining({ fields: filter.fields }),
  );
});

test.each([
  { state: "pending", label: "Pending", deliveryCheckPending: false },
  { state: "uncertain", label: "Unconfirmed", deliveryCheckPending: true },
])(
  "older Activity pages poll $state deliveries while work is active",
  async ({ state, label, deliveryCheckPending }) => {
    vi.useFakeTimers();
    let requests = 0;
    const queued = {
      ...entry,
      key: "integration_message:1",
      kind: "notification",
      title: "Queued delivery",
      state,
    };
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      if (String(input).includes("/filters?")) return Response.json(choices);
      requests++;
      return Response.json({
        items: [
          {
            ...queued,
            state: requests >= 4 ? "delivered" : state,
            delivery_check_pending: deliveryCheckPending && requests < 4,
          },
        ],
        next: "",
      });
    });
    await act(async () => {
      start({ before: "older-page" });
    });
    expect(screen.getByRole("table").textContent).toContain(label);
    const initial = requests;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(requests).toBeGreaterThan(initial);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000);
    });
    expect(screen.getByRole("table").textContent).toContain("Delivered");
    const finished = requests;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(35000);
    });
    expect(requests).toBe(finished);
  },
);

test("Activity offers source filters and discovers choices only for game events", async () => {
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async (input) =>
      Response.json(
        String(input).includes("/filters?")
          ? choices
          : { items: [entry], next: "" },
      ),
    );
  start();
  await screen.findByRole("button", { name: event.title });
  const discovery = () =>
    fetch.mock.calls.filter(([input]) => String(input).includes("/filters?"));
  expect(discovery()).toHaveLength(0);
  const user = userEvent.setup();
  await user.selectOptions(
    screen.getByLabelText("Activity type"),
    "operations",
  );
  await user.selectOptions(screen.getByLabelText("Activity type"), "alert");
  expect(discovery()).toHaveLength(0);
  await user.selectOptions(screen.getByLabelText("Activity type"), "game");
  await waitFor(() => expect(discovery()).toHaveLength(1));
  expect(String(discovery()[0]![0])).toContain("kind=game");
  expect(screen.queryByRole("option", { name: "Notifications" })).toBeNull();
  expect(screen.queryByLabelText("Provider")).toBeNull();
  expect(screen.queryByLabelText("Destination")).toBeNull();
  await user.selectOptions(screen.getByLabelText("Activity type"), "");
  expect(discovery()).toHaveLength(1);
  expect(
    activityFilterFromQuery(
      new URLSearchParams(
        "kind=notification&provider=discord&destination=400&source=game_event:3&state=failed",
      ),
    ),
  ).toEqual({ kind: "" });
});

test("operation retry requires confirmation and opens the new attempt", async () => {
  const onOpen = vi.fn();
  const onChanged = vi.fn(async () => {});
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async (_input, init) =>
      Response.json(
        init?.method === "POST"
          ? {
              ...operation,
              id: "new",
              retry_of: "old",
              state: "running",
              message: "",
            }
          : {
              ...entry,
              key: "operation:old",
              kind: "operation",
              title: "restart",
              state: "failed",
              operation,
              can_retry_operation: true,
            },
      ),
    );
  render(
    <ActivityDetailsDrawer
      server={server}
      entryKey="operation:old"
      onClose={vi.fn()}
      onOpen={onOpen}
      onChanged={onChanged}
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: "Try again" }));
  const confirmation = await screen.findByRole("dialog", {
    name: "Retry operation?",
  });
  const posts = () =>
    fetch.mock.calls.filter(([, init]) => init?.method === "POST");
  expect(posts()).toHaveLength(0);
  fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));
  expect(posts()).toHaveLength(0);
  fireEvent.click(screen.getByRole("button", { name: "Try again" }));
  fireEvent.click(
    within(
      await screen.findByRole("dialog", { name: "Retry operation?" }),
    ).getByRole("button", { name: "Try again" }),
  );
  await waitFor(() => expect(onOpen).toHaveBeenCalledWith("operation:new"));
  expect(posts()).toHaveLength(1);
  expect(onChanged).toHaveBeenCalledTimes(1);
});

test("older related notifications load inside the source drawer without changing feed filters", async () => {
  const message = {
    ...entry,
    key: "notification:1",
    kind: "notification",
    title: "Newer message",
    state: "created",
    source_key: entry.key,
    source_available: true,
  };
  const older = { ...message, key: "notification:2", title: "Older message" };
  const requests: URL[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = new URL(String(input), "https://panel.test");
    if (url.pathname.endsWith("/filters")) return Response.json(choices);
    if (url.pathname.endsWith("/notifications")) {
      requests.push(url);
      return Response.json(
        url.searchParams.has("before")
          ? { items: [older], next: "" }
          : { items: [message], next: "related-page" },
      );
    }
    if (url.pathname.endsWith("/activity")) {
      return Response.json({ items: [entry], next: "" });
    }
    return Response.json({
      ...details,
      notification_count: 51,
    });
  });
  start({
    kind: "game",
    type: "player.joined",
    fields: { player: "Morgan" },
    search: "Morgan",
    from: event.received_at - 1,
    to: event.received_at + 1,
    before: "older-page",
  });
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: event.title }));
  const route = window.location.hash;
  await userEvent
    .setup()
    .click(
      await screen.findByRole("button", { name: "Load older notifications" }),
    );
  await screen.findByRole("button", { name: "Older message" });
  expect(screen.getByRole("button", { name: "Newer message" })).toBeDefined();
  expect(window.location.hash).toBe(route);
  expect(
    screen.queryByRole("button", { name: "Load older notifications" }),
  ).toBeNull();
  expect(requests[0]?.search).toBe("");
  expect(
    requests.every(
      (url) =>
        url.pathname ===
        `/api/v1/servers/${server.id}/activity/${encodeURIComponent(entry.key)}/notifications`,
    ),
  ).toBe(true);
  expect(requests.at(-1)?.searchParams.get("before")).toBe("related-page");
  expect(requests.at(-1)?.searchParams.has("kind")).toBe(false);
});

test.each(["request", "refresh", "navigation"])(
  "leaving during the retry %s prevents late navigation and drawer refresh",
  async (stage) => {
    replaceCurrentRoute("servers/game/activity?entry=operation%3Aold");
    let finishRequest!: (response: Response) => void;
    let finishRefresh!: () => void;
    const response = new Promise<Response>((resolve) => {
      finishRequest = resolve;
    });
    const refreshed = new Promise<void>((resolve) => {
      finishRefresh = resolve;
    });
    const onOpen = vi.fn();
    const onChanged = vi.fn(() =>
      stage === "refresh" ? refreshed : Promise.resolve(),
    );
    const fetch = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(async (_input, init) => {
        if (init?.method === "POST") return response;
        return Response.json({
          ...entry,
          key: "operation:old",
          kind: "operation",
          title: "restart",
          state: "failed",
          operation,
          can_retry_operation: true,
        });
      });
    const view = render(
      <StrictMode>
        <ActivityDetailsDrawer
          server={server}
          entryKey="operation:old"
          onClose={vi.fn()}
          onOpen={onOpen}
          onChanged={onChanged}
        />
      </StrictMode>,
    );
    fireEvent.click(await screen.findByRole("button", { name: "Try again" }));
    fireEvent.click(
      within(
        await screen.findByRole("dialog", { name: "Retry operation?" }),
      ).getByRole("button", { name: "Try again" }),
    );
    await waitFor(() =>
      expect(fetch.mock.calls.some(([, init]) => init?.method === "POST")).toBe(
        true,
      ),
    );
    const finish = () =>
      finishRequest(
        Response.json({
          ...operation,
          id: "new",
          retry_of: "old",
          state: "running",
          message: "",
        }),
      );
    if (stage === "refresh") {
      await act(async () => {
        finish();
      });
      expect(onChanged).toHaveBeenCalledTimes(1);
    }
    if (stage === "navigation") replaceCurrentRoute("dashboard");
    else view.unmount();
    const requests = fetch.mock.calls.length;
    await act(async () => {
      if (stage === "refresh") finishRefresh();
      else finish();
    });
    expect(onOpen).not.toHaveBeenCalled();
    expect(onChanged).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledTimes(requests);
  },
);

test.each([
  {
    navigation: "stays on the original page",
    pages: [{ before: "older-page" }],
  },
  {
    navigation: "moves to another page",
    pages: [{ before: "different-page" }],
  },
  {
    navigation: "returns to the original page",
    pages: [{ before: "different-page" }, { before: "older-page" }],
  },
  {
    navigation: "changes filters on an older page",
    pages: [{ before: "older-page", search: "Server" }],
  },
])(
  "retry completion respects the current feed when the user $navigation",
  async ({ pages }) => {
    const item = {
      ...entry,
      key: "integration_message:1",
      kind: "notification",
      title: "Server report",
      state: "failed",
    };
    const filter = { before: "older-page" };
    const reads: URL[] = [];
    let state = item.state;
    let finish!: (response: Response) => void;
    const response = new Promise<Response>((resolve) => {
      finish = resolve;
    });
    const fetch = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(async (input, init) => {
        const url = new URL(String(input), "https://panel.test");
        if (init?.method === "POST") return response;
        if (url.pathname.endsWith("/filters")) return Response.json(choices);
        if (url.pathname.endsWith("/activity")) {
          reads.push(url);
          return Response.json({ items: [{ ...item, state }], next: "" });
        }
        return Response.json({
          ...item,
          can_retry_operation: false,
          notification: {
            message_id: 1,
            name: "Discord",
            body: "",
            attempts: 8,
            next_attempt_at: 0,
            delivered_at: 0,
            error: "Delivery failed",
            can_retry: true,
          },
        });
      });
    replaceCurrentRoute(
      serverActivityURL(server.id, filter, item.key).slice(1),
    );
    render(<ServerActivity server={server} />);
    const drawer = await screen.findByRole("dialog", { name: item.title });
    fireEvent.click(within(drawer).getByRole("button", { name: "Try again" }));
    expect(fetch.mock.calls.some(([, init]) => init?.method === "POST")).toBe(
      true,
    );
    let currentFilter: ActivityFilter = filter;
    for (const pageFilter of pages) {
      currentFilter = { ...filter, ...pageFilter };
      await act(async () => {
        replaceCurrentRoute(
          serverActivityURL(server.id, currentFilter).slice(1),
        );
      });
    }
    expect(screen.queryByRole("dialog")).toBeNull();
    const previousReads = reads.length;
    await act(async () => {
      state = "delivered";
      finish(new Response(null, { status: 204 }));
    });
    expect(reads).toHaveLength(previousReads + 1);
    expect(reads.at(-1)?.searchParams.get("before")).toBe(currentFilter.before);
    expect(reads.at(-1)?.searchParams.get("search")).toBe(
      currentFilter.search ?? null,
    );
    expect(screen.getByRole("table").textContent).toContain("Delivered");
    expect(screen.queryByRole("dialog")).toBeNull();
  },
);
