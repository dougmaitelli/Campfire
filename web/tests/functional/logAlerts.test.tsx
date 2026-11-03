import { beforeEach, expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ServerLogAlerts } from "../../src/components/servers/ServerLogAlerts";
import { listLogAlerts, dismissLogAlert } from "../../src/api/logAlerts";
import type { Server } from "../../src/api/servers";
import type { DetectedLogAlert } from "../../src/contracts/logAlerts";

vi.mock("../../src/api/logAlerts", () => ({
  listLogAlerts: vi.fn(),
  dismissLogAlert: vi.fn(),
}));

vi.mock("../../src/components/servers/ServerSettingsDrawer", () => ({
  ServerSettingsDrawer: () => (
    <input aria-label="Pending settings" defaultValue="" />
  ),
}));

const alert: DetectedLogAlert = {
  id: "a".repeat(64),
  rule_key: "b".repeat(64),
  execution_id: "c".repeat(64),
  phase: "runtime",
  title: "EULA requires acceptance",
  message: "Review the EULA before changing eula.txt.",
  severity: "error",
  detected_at: 1792670400,
  line: "You need to agree to the EULA",
  action: { type: "open_file", path: "eula.txt" },
};

function server(permissions: Server["permissions"]): Server {
  return {
    id: "game",
    name: "Game",
    description: "",
    spark_id: "node",
    ember_id: "ember",
    image: "game:latest",
    variables: {},
    cpu_limit: 1,
    memory_mb: 1024,
    disk_mb: 0,
    ports: [],
    ember_hash: "hash",
    ember_update_available: false,
    owner_user_id: 1,
    permissions,
    spark_name: "Node",
    ember_name: "Game",
    game_address: "",
    status: "running",
    created_at: "2026-10-22",
  };
}

beforeEach(() => {
  vi.mocked(listLogAlerts).mockResolvedValue([alert]);
  vi.mocked(dismissLogAlert).mockResolvedValue();
});

test("an issue exposes guidance, evidence and only authorized actions", async () => {
  render(
    <ServerLogAlerts
      server={server(["server.view"])}
      busy={false}
      onServerSaved={vi.fn()}
    />,
  );
  await screen.findByText(alert.title);
  expect(screen.getByText(alert.message)).toBeDefined();
  expect(screen.queryByText("Matched console output")).toBeNull();
  expect(screen.queryByRole("link", { name: "Open eula.txt" })).toBeNull();
  expect(screen.queryByRole("link", { name: "View console" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
});

test("authorized users can inspect evidence, navigate to the file and dismiss a detection", async () => {
  const user = userEvent.setup();
  render(
    <ServerLogAlerts
      server={server([
        "server.view",
        "server.operate",
        "server.console.read",
        "server.files.read",
      ])}
      busy={false}
      onServerSaved={vi.fn()}
    />,
  );
  await screen.findByText(alert.title);
  expect(
    screen.getByRole("link", { name: "Open eula.txt" }).getAttribute("href"),
  ).toBe("#servers/game/files/eula.txt");
  await user.click(screen.getByText("Matched console output"));
  expect(screen.getByText(alert.line!)).toBeDefined();
  vi.mocked(listLogAlerts).mockResolvedValue([]);
  await user.click(screen.getByRole("button", { name: "Dismiss" }));
  await waitFor(() => expect(screen.queryByText(alert.title)).toBeNull());
  expect(dismissLogAlert).toHaveBeenCalledWith("game", alert.id);
});

test("an empty issue list adds no empty panel", async () => {
  vi.mocked(listLogAlerts).mockResolvedValue([]);
  render(
    <ServerLogAlerts
      server={server(["server.view"])}
      busy={false}
      onServerSaved={vi.fn()}
    />,
  );
  await waitFor(() => expect(listLogAlerts).toHaveBeenCalled());
  expect(screen.queryByRole("region", { name: "Needs attention" })).toBeNull();
});

test("polling away the final alert preserves unsaved server settings", async () => {
  vi.mocked(listLogAlerts).mockResolvedValue([
    { ...alert, action: { type: "server_settings" } },
  ]);
  const user = userEvent.setup();
  render(
    <ServerLogAlerts
      server={server(["server.view", "server.configure"])}
      busy={false}
      onServerSaved={vi.fn()}
    />,
  );
  await user.click(
    await screen.findByRole("button", { name: "Server settings" }),
  );
  const input = await screen.findByRole("textbox", {
    name: "Pending settings",
  });
  await user.type(input, "unsaved change");
  vi.mocked(listLogAlerts).mockResolvedValue([]);
  await waitFor(
    () =>
      expect(
        screen.queryByRole("region", { name: "Needs attention" }),
      ).toBeNull(),
    { timeout: 4500 },
  );
  expect(
    screen.getByRole<HTMLInputElement>("textbox", { name: "Pending settings" })
      .value,
  ).toBe("unsaved change");
});
