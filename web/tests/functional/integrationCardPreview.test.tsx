import { render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import {
  previewIntegrationCard,
  type IntegrationCard,
} from "../../src/api/integrations";
import { IntegrationCardPreview } from "../../src/components/servers/IntegrationCardPreview";
import { RuntimeState } from "../../src/contracts/domains";
import i18n from "../../src/i18n";

vi.mock("../../src/api/integrations", () => ({
  previewIntegrationCard: vi.fn(),
}));

const card: IntegrationCard = {
  name: "Game server",
  state: RuntimeState.Running,
  addresses: ["games.example.test:25565"],
  expires_at: 0,
  fields: [
    {
      id: "players",
      name: "Players",
      value: "3",
      available: true,
    },
  ],
  preview: {
    embeds: [
      {
        title: "Game server",
        fields: [
          { name: "Status", value: "Running" },
          { name: "Address", value: "games.example.test:25565" },
          { name: "Players", value: "3" },
        ],
        footer: { text: "Campfire server" },
      },
    ],
  },
  expired_preview: {
    embeds: [
      {
        title: "Game server",
        fields: [
          { name: "Status", value: "Unknown" },
          { name: "Address", value: "games.example.test:25565" },
          { name: "Players", value: "Unavailable" },
        ],
        footer: { text: "Campfire server" },
      },
    ],
  },
};

test("preview consumes the actual Discord presentation without adding formatting", async () => {
  const response = structuredClone(card);
  response.preview.embeds[0]!.fields.push({
    name: "Server.status…",
    value: "truncated value…",
  });
  response.fields.push({
    id: "field.details",
    name: "Server status",
    value: "unbounded value",
    available: true,
  });
  vi.mocked(previewIntegrationCard).mockResolvedValue(response);
  const { container } = render(
    <IntegrationCardPreview
      serverID="server"
      fields={["players", "field.details"]}
    />,
  );
  await screen.findByText("Game server");
  expect(container.querySelector("dd")?.textContent).toBe("Running");
  expect(screen.getByText("Server.status…")).toBeTruthy();
  expect(screen.getByText("truncated value…")).toBeTruthy();
  expect(screen.queryByText("unbounded value")).toBeNull();
  expect(container.querySelector(".status-pill")).toBeNull();
});

test("expired preview uses the server's unknown presentation and hides stale values", async () => {
  vi.mocked(previewIntegrationCard).mockResolvedValue({
    ...card,
    expires_at: 1,
  });
  const { container } = render(
    <IntegrationCardPreview serverID="server" fields={["players"]} />,
  );
  await screen.findByText("Game server");
  expect(container.querySelector("dd")?.textContent).toBe("Unknown");
  expect(screen.getByText("games.example.test:25565")).toBeTruthy();
  expect(screen.getByText("Unavailable")).toBeTruthy();
  expect(screen.queryByText("3")).toBeNull();
});

test("preview preserves Discord embed continuation and English publication locale", async () => {
  i18n.addResourceBundle("test", "translation", {
    gameState: { players: "Connected players" },
  });
  await i18n.changeLanguage("test");
  const response = structuredClone(card);
  response.preview.embeds.push({
    title: "Additional server details",
    fields: [{ name: "World name", value: "Meadows" }],
    footer: { text: "Campfire server" },
  });
  vi.mocked(previewIntegrationCard).mockResolvedValue(response);
  const { container } = render(
    <IntegrationCardPreview serverID="server" fields={["players"]} />,
  );
  await screen.findByText("Additional server details");
  expect(container.querySelectorAll(".integration-server-card")).toHaveLength(
    2,
  );
  expect(screen.getByText("Players")).toBeTruthy();
  expect(screen.getByText("World name")).toBeTruthy();
  expect(screen.queryByText("Connected players")).toBeNull();
});
