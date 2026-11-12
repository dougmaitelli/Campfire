import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import {
  listServerDiscord,
  serverDiscordChannels,
} from "../../src/api/integrations";
import type { Server } from "../../src/api/servers";
import { ServerIntegrationsStep } from "../../src/components/servers/ServerIntegrationsStep";
import { Wizard } from "../../src/components/wizard/Wizard";
import { useServerIntegrations } from "../../src/hooks/useServerIntegrations";

vi.mock("../../src/api/integrations", () => ({
  listServerDiscord: vi.fn(),
  serverDiscordChannels: vi.fn(),
  saveServerIntegrationChanges: vi.fn(),
}));

const server = {
  id: "server",
  permissions: [],
  game_integration: false,
} as unknown as Server;

function IntegrationWizard() {
  const state = useServerIntegrations(server, true);
  return (
    <Wizard
      busy={false}
      error={null}
      finishLabel="Save"
      onFinish={state.save}
      steps={[
        {
          id: "integrations",
          title: "Integrations",
          validate: state.validate,
          content: (
            <ServerIntegrationsStep
              server={server}
              state={state}
              busy={false}
            />
          ),
        },
      ]}
    >
      {(content, actions) => (
        <>
          {content}
          {actions}
        </>
      )}
    </Wizard>
  );
}

test.each([1, 2])(
  "adding a draft with %s connections does not immediately show an error",
  async (connectionCount) => {
    vi.mocked(listServerDiscord).mockResolvedValue({
      configured: true,
      available: true,
      guild_name: "Community",
      sources: [],
      connections: Array.from({ length: connectionCount }, (_, index) => ({
        id: `discord-${index}`,
        provider: "discord",
        name: `Community ${index}`,
        enabled: true,
      })),
      items: [],
    });
    vi.mocked(serverDiscordChannels).mockResolvedValue([
      { id: "400", name: "public", available: true, missing: [] },
      {
        id: "401",
        name: "restricted",
        available: false,
        missing: ["Send Messages"],
      },
    ]);
    const user = userEvent.setup();
    render(<IntegrationWizard />);
    await user.click(
      await screen.findByRole("button", { name: "Add integration" }),
    );
    const save = screen.getByRole<HTMLButtonElement>("button", {
      name: "Save",
    });
    await waitFor(() => expect(save.disabled).toBe(true));
    expect(screen.queryByRole("alert")).toBeNull();

    // Radix exposes native select controls for form validity and autofill.
    const connection = document.querySelector<HTMLSelectElement>("select");
    expect(connection?.required).toBe(true);
    if (connectionCount > 1) {
      fireEvent.change(connection!, { target: { value: "discord-0" } });
      await screen.findByRole("combobox", { name: "Discord channel" });
      expect(screen.queryByRole("alert")).toBeNull();
      expect(save.disabled).toBe(true);
    }
    const channel = document.querySelectorAll<HTMLSelectElement>("select")[1]!;
    expect(channel.required).toBe(true);
    await waitFor(() =>
      expect(channel.querySelector('option[value="400"]')).not.toBeNull(),
    );
    fireEvent.change(channel, { target: { value: "400" } });
    await waitFor(() => expect(save.disabled).toBe(false));
    expect(screen.queryByRole("alert")).toBeNull();

    fireEvent.change(channel, { target: { value: "401" } });
    await waitFor(() => expect(save.disabled).toBe(true));
    expect(screen.getByRole("alert").textContent).toContain(
      "Choose an available integration and channel",
    );
  },
);
