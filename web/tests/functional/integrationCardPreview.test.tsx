import { render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { previewIntegrationCard } from "../../src/api/integrations";
import { IntegrationCardPreview } from "../../src/components/servers/IntegrationCardPreview";
import { RuntimeState } from "../../src/contracts/domains";

vi.mock("../../src/api/integrations", () => ({
  previewIntegrationCard: vi.fn(),
}));

const statuses: [RuntimeState, string][] = [
  [RuntimeState.Running, "🟢 running"],
  [RuntimeState.Stopped, "⚪ stopped"],
  [RuntimeState.Exited, "⚪ exited"],
  [RuntimeState.Dead, "🔴 dead"],
  [RuntimeState.Unknown, "🔴 unknown"],
  [RuntimeState.NotInstalled, "🟠 not installed"],
  [RuntimeState.Paused, "🟠 paused"],
  [RuntimeState.Starting, "🟠 starting"],
  [RuntimeState.Stopping, "🟠 stopping"],
  [RuntimeState.Restarting, "🟠 restarting"],
  [RuntimeState.Installing, "🟠 installing"],
  [RuntimeState.Updating, "🟠 updating"],
  [RuntimeState.Reinstalling, "🟠 reinstalling"],
  [RuntimeState.Deleting, "🟠 deleting"],
];

test.each(statuses)(
  "preview uses Discord's emoji and text for %s",
  async (state, expected) => {
    vi.mocked(previewIntegrationCard).mockResolvedValue({
      name: "Game server",
      state,
      addresses: [],
      expires_at: 0,
      fields: [],
    });
    const { container } = render(
      <IntegrationCardPreview serverID="server" fields={[]} />,
    );
    await screen.findByText("Game server");
    expect(container.querySelector("dd")?.textContent).toBe(expected);
    expect(container.querySelector(".status-pill")).toBeNull();
  },
);

test("expired preview uses Discord's unknown indicator and hides stale values", async () => {
  vi.mocked(previewIntegrationCard).mockResolvedValue({
    name: "Game server",
    state: RuntimeState.Running,
    addresses: ["games.example.test:25565"],
    expires_at: 1,
    fields: [{ id: "players", name: "Players", value: "3", available: true }],
  });
  const { container } = render(
    <IntegrationCardPreview serverID="server" fields={["players"]} />,
  );
  await screen.findByText("Game server");
  expect(container.querySelector("dd")?.textContent).toBe("🔴 unknown");
  expect(screen.getByText("games.example.test:25565")).toBeTruthy();
  expect(screen.queryByText("3")).toBeNull();
});
