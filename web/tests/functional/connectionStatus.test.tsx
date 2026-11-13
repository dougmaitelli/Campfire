import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  ConnectionNotice,
  ConnectionStatusProvider,
} from "../../src/components/status/ConnectionStatus";
import { ServerLogAlerts } from "../../src/components/servers/ServerLogAlerts";
import { ServerActivity } from "../../src/components/servers/ServerActivity";
import { InlineMessage } from "../../src/components/status/InlineMessage";
import { useMonitoring } from "../../src/hooks/useMonitoring";
import { useCollection } from "../../src/hooks/useCollection";
import { apiFetch, readJSON } from "../../src/api/client";
import type { Server } from "../../src/api/servers";
import type { DetectedLogAlert } from "../../src/contracts/logAlerts";

const notice = "Connection to Campfire interrupted. Retrying…";
const server: Server = {
  id: "game",
  name: "Survival",
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
  permissions: ["server.view", "server.operate"],
  spark_name: "Node",
  ember_name: "Game",
  game_address: "",
  status: "running",
  created_at: "2026-10-25",
};
const entry = {
  key: "game_event:3",
  kind: "game",
  source_available: false,
  created_at: 1792800000,
  title: "Game integration connected",
  state: "",
  notification_count: 0,
};
const alert: DetectedLogAlert = {
  id: "a".repeat(64),
  rule_key: "b".repeat(64),
  execution_id: "c".repeat(64),
  phase: "runtime",
  title: "EULA requires acceptance",
  message: "Review the EULA before changing eula.txt.",
  severity: "error",
  detected_at: 1792670400,
};
let failed: Set<string>;
let alerts: DetectedLogAlert[];
let forbidden: boolean;

function Monitoring() {
  const monitoring = useMonitoring();
  return (
    <>
      {monitoring.error && <InlineMessage>{monitoring.error}</InlineMessage>}
      <output aria-label="Cached monitoring timestamp">
        {monitoring.snapshot?.collected_at}
      </output>
    </>
  );
}
function Harness({ activity = true }: { activity?: boolean }) {
  return (
    <ConnectionStatusProvider>
      <ConnectionNotice />
      <Monitoring />
      <ServerLogAlerts server={server} busy={false} onServerSaved={vi.fn()} />
      {activity && <ServerActivity server={server} />}
    </ConnectionStatusProvider>
  );
}
async function advance(milliseconds: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(milliseconds);
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  window.location.hash = "servers/game/activity";
  failed = new Set();
  alerts = [];
  forbidden = false;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const path = new URL(String(input), "https://panel.test").pathname;
    const kind = path.split("/").at(-1)!;
    if (failed.has(kind))
      throw new TypeError("NetworkError when attempting to fetch resource.");
    if (kind === "monitoring")
      return Response.json({
        sample_interval_seconds: 10,
        collected_at: 1792800000,
        incomplete: false,
        servers: [],
      });
    if (kind === "log-alerts")
      return forbidden
        ? Response.json(
            { error: "You cannot read log alerts." },
            { status: 403 },
          )
        : Response.json(alerts);
    if (kind === "activity") return Response.json({ items: [entry], next: "" });
    if (kind === "dismiss") return new Response(null, { status: 204 });
    throw new Error(`Unexpected request: ${path}`);
  });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

test("simultaneous background failures produce one notice and retain cached content", async () => {
  await act(async () => {
    render(<Harness />);
  });
  expect(screen.getByText(entry.title)).toBeDefined();
  failed = new Set(["monitoring", "log-alerts", "activity"]);
  await advance(10000);
  expect(screen.getAllByRole("alert")).toHaveLength(1);
  expect(screen.getByText(notice)).toBeDefined();
  expect(screen.queryByRole("region", { name: "Needs attention" })).toBeNull();
  expect(screen.getByText(entry.title)).toBeDefined();
  expect(screen.getByLabelText("Cached monitoring timestamp").textContent).toBe(
    "1792800000",
  );
  failed.clear();
  await advance(10000);
  expect(screen.queryByText(notice)).toBeNull();
  expect(screen.getByText(entry.title)).toBeDefined();
});

test("successful sibling requests do not clear another reader's failure, but unmounting does", async () => {
  let view!: ReturnType<typeof render>;
  await act(async () => {
    view = render(<Harness />);
  });
  failed = new Set(["monitoring", "log-alerts", "activity"]);
  await advance(10000);
  failed = new Set(["activity"]);
  await advance(10000);
  expect(screen.getAllByText(notice)).toHaveLength(1);
  await act(async () => {
    view.rerender(<Harness activity={false} />);
  });
  expect(screen.queryByText(notice)).toBeNull();
});

test("API permission errors remain visible alongside the shared connection notice", async () => {
  await act(async () => {
    render(<Harness />);
  });
  failed.add("monitoring");
  forbidden = true;
  await advance(10000);
  expect(screen.getByText(notice)).toBeDefined();
  expect(screen.getByText("You cannot read log alerts.")).toBeDefined();
  expect(screen.getByRole("region", { name: "Needs attention" })).toBeDefined();
  expect(screen.getAllByRole("alert")).toHaveLength(2);
});

test("an initial outage shows unavailable data rather than an empty result or endless spinner", async () => {
  failed = new Set(["monitoring", "log-alerts", "activity"]);
  await act(async () => {
    render(<Harness />);
  });
  expect(screen.getAllByRole("alert")).toHaveLength(1);
  expect(
    screen.getByText("Data unavailable while reconnecting."),
  ).toBeDefined();
  expect(screen.queryByText("No activity matches these filters.")).toBeNull();
  expect(document.querySelector('[aria-busy="true"]')).toBeNull();
  expect(screen.queryByRole("region", { name: "Needs attention" })).toBeNull();
});

test("real cached log alerts remain visible during a connection outage", async () => {
  alerts = [alert];
  await act(async () => {
    render(<Harness />);
  });
  failed = new Set(["monitoring", "log-alerts", "activity"]);
  await advance(10000);
  expect(screen.getByText(notice)).toBeDefined();
  expect(screen.getByText(alert.title)).toBeDefined();
  expect(screen.getByRole("region", { name: "Needs attention" })).toBeDefined();
});

test("failed user actions stay local and are not automatically retried", async () => {
  alerts = [alert];
  await act(async () => {
    render(<Harness />);
  });
  failed.add("dismiss");
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
  });
  expect(screen.getByText("Connection to Campfire interrupted.")).toBeDefined();
  expect(screen.queryByText(notice)).toBeNull();
  await advance(10000);
  expect(
    vi
      .mocked(fetch)
      .mock.calls.filter(([url]) => String(url).endsWith("/dismiss")),
  ).toHaveLength(1);
});

const readOnce = async (signal: AbortSignal) => {
  await readJSON(await apiFetch("/one-shot", { signal }));
  return [{ id: "recovered", name: "Recovered item" }];
};
function OneShotReader() {
  const collection = useCollection(readOnce);
  return (
    <>
      {collection.items.map((item) => (
        <p key={item.id}>{item.name}</p>
      ))}
    </>
  );
}
test("one-shot read failures retry, keeping the notice visible until recovery completes", async () => {
  vi.mocked(fetch).mockRejectedValueOnce(new TypeError("Failed to fetch"));
  await act(async () => {
    render(
      <ConnectionStatusProvider>
        <ConnectionNotice />
        <OneShotReader />
      </ConnectionStatusProvider>,
    );
  });
  expect(screen.getByText(notice)).toBeDefined();
  let complete!: (response: Response) => void;
  vi.mocked(fetch).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  await advance(3000);
  expect(screen.getByText(notice)).toBeDefined();
  await act(async () => {
    complete(Response.json([]));
  });
  expect(screen.queryByText(notice)).toBeNull();
  expect(screen.getByText("Recovered item")).toBeDefined();
});
