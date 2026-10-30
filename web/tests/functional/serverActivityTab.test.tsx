import { expect, test, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ServerOperationsPage } from "../../src/pages/ServerOperationsPage";
import type { Server } from "../../src/api/servers";

vi.mock("../../src/hooks/useAuthentication", () => ({
  useAuthentication: () => ({ session: null }),
}));
vi.mock("../../src/hooks/useServerRuntime", () => ({
  useServerRuntime: () => ({ statuses: {} }),
}));
vi.mock("../../src/hooks/useMonitoring", () => ({
  useMonitoring: () => ({ snapshot: null, elapsed: 0 }),
}));
vi.mock("../../src/hooks/useResourceHistory", () => ({
  useResourceHistory: () => null,
}));
vi.mock("../../src/drawers/ServerConsoleDrawer", () => ({
  ServerConsoleDrawer: () => <div>Console content</div>,
}));
vi.mock("../../src/drawers/ServerFilesDrawer", () => ({
  ServerFilesDrawer: () => null,
}));
vi.mock("../../src/drawers/ServerBackupsDrawer", () => ({
  ServerBackupsDrawer: () => null,
}));
vi.mock("../../src/components/servers/ServerGameEvents", () => ({
  ServerGameEvents: () => <div>Activity content</div>,
}));

test.each([true, false, undefined])(
  "activity visibility follows game integration setting %s, including direct links",
  async (enabled) => {
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
      permissions: ["server.view", "server.operate", "server.console.read"],
      spark_name: "Node",
      ember_name: "Game",
      game_address: "",
      status: "running",
      created_at: "2026-10-25",
      ...(enabled === undefined ? {} : { game_integration: enabled }),
    };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json(server));
    render(<ServerOperationsPage serverID="game" tab="activity" />);
    await screen.findByRole("heading", { name: "Test server" });
    expect(screen.queryByRole("link", { name: "Game activity" }) !== null).toBe(
      enabled === true,
    );
    if (enabled) {
      expect(screen.getByText("Activity content")).toBeDefined();
      expect(screen.queryByText("Console content")).toBeNull();
    } else {
      expect(screen.queryByText("Activity content")).toBeNull();
      expect(screen.getByText("Console content")).toBeDefined();
    }
  },
);
