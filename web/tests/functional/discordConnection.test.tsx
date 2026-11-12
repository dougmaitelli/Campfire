import { fireEvent, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { previewDiscord } from "../../src/api/integrations";
import { DiscordConnectionForm } from "../../src/components/integrations/DiscordConnectionForm";

vi.mock("../../src/api/integrations", () => ({
  previewDiscord: vi.fn(),
  discordChannels: vi.fn(),
  saveDiscord: vi.fn(),
  testDiscord: vi.fn(),
}));

test.each([undefined, "Discord servers could not be loaded."])(
  "new bots retain their invite link with no guilds (error: %s)",
  async (guildError) => {
    const invite =
      "https://discord.com/oauth2/authorize?client_id=100&scope=bot";
    vi.mocked(previewDiscord).mockResolvedValue({
      bot: { id: "100", username: "new-bot" },
      guilds: [],
      invite_url: invite,
      guild_error: guildError,
    });
    const onStateChange = vi.fn();
    render(
      <DiscordConnectionForm
        current={null}
        formID="discord"
        onStateChange={onStateChange}
        onSaved={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByLabelText("Bot token"), {
      target: { value: "private-token" },
    });
    const link = await screen.findByRole("link", {
      name: "Add bot to Discord",
    });
    expect(link.getAttribute("href")).toBe(invite);
    expect(link.getAttribute("href")).not.toContain("private-token");
    expect(screen.getByRole("alert").textContent).toContain(
      guildError ?? "The bot has no Discord servers.",
    );
    expect(onStateChange).toHaveBeenLastCalledWith({
      busy: false,
      canSave: false,
    });
  },
);
