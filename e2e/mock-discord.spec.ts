import { test, expect } from "./fixtures";
import { login } from "./screens";

test("mock Discord shows real deliveries, edits one card, and recreates a deleted card", async ({
  page,
  baseURL,
}) => {
  test.setTimeout(60_000);
  await login(page);
  const { csrf_token } = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  const headers = { "X-CSRF-Token": csrf_token, Origin: baseURL! };
  expect(
    (
      await page.request.put("/api/v1/integrations/discord", {
        headers,
        data: {
          enabled: true,
          guild_id: "200",
          token: "campfire-test-discord-token",
        },
      })
    ).ok(),
  ).toBeTruthy();
  expect(
    (
      await page.request.post("/api/v1/integrations/discord/test", {
        headers,
        data: { channel_id: "400" },
      })
    ).ok(),
  ).toBeTruthy();
  let bindingID = "";
  const saveCard = async (fields: string[]) => {
    const response = await page.request.fetch(
      `/api/v1/servers/survival/integrations/discord${bindingID ? `/${bindingID}` : ""}`,
      {
        method: bindingID ? "PUT" : "POST",
        headers,
        data: {
          enabled: true,
          channel_id: "400",
          subscriptions: [{ category: "game" }],
          card: { enabled: true, fields },
        },
      },
    );
    expect(response.ok()).toBeTruthy();
    bindingID = ((await response.json()) as { id: string }).id;
  };
  await saveCard([]);
  await expect
    .poll(async () => {
      const settings = (await (
        await page.request.get(
          `/api/v1/servers/survival/integrations/discord/${bindingID}`,
        )
      ).json()) as { card_status: { state: string } };
      return settings.card_status.state;
    })
    .toBe("published");
  expect(
    (
      await page.request.post("/__test/integrations/event", {
        headers: { "X-Campfire-E2E": "integration" },
        data: {
          id: "game:mock-viewer",
          server_id: "survival",
          category: "game",
          type: "player.joined",
          title: "Alex joined the server",
          occurred_at: 1792670400,
          data: { player: "Alex", state: "connected" },
        },
      })
    ).ok(),
  ).toBeTruthy();

  await page.goto("/mock/discord");
  await expect(
    page.getByRole("heading", { name: "Mock Discord", exact: true }),
  ).toBeVisible();
  await expect(page.locator("#token")).toHaveText(
    "campfire-test-discord-token",
  );
  const card = page.getByRole("article").filter({
    has: page.getByRole("heading", { name: "Survival", exact: true }),
  });
  await expect(card).toHaveCount(1);
  await expect(card.locator(".embed")).toHaveCSS(
    "border-left-color",
    "rgb(128, 185, 118)",
  );
  await expect(
    page.getByText("Campfire integration is ready. This is a test message.", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", {
      name: "Survival: Alex joined the server",
      exact: true,
    }),
  ).toBeVisible();
  const originalID = await card.getAttribute("data-message-id");
  await saveCard(["players"]);
  await expect(card).toContainText("players");
  await expect(card).toContainText("edited");
  await expect(card).toHaveAttribute("data-message-id", originalID!);
  await card
    .getByRole("button", { name: "Delete message", exact: true })
    .click();
  await expect(card).toHaveCount(0);
  // Changing selected fields makes the next reconciliation detect the deletion
  // without waiting for the quiet card's one-minute existence check.
  await saveCard([]);
  await expect(card).toHaveCount(1, { timeout: 20_000 });
  await expect(card).not.toHaveAttribute("data-message-id", originalID!);
  await expect(page.getByRole("article")).toHaveCount(3);
});

test("mock Discord channel controls stay usable on mobile and isolate failures", async ({
  page,
}) => {
  await page.goto("/mock/discord");
  const behavior = page.getByRole("combobox", {
    name: "Channel behavior",
    exact: true,
  });
  await expect(behavior).toHaveValue("");
  await behavior.selectOption("forbidden");
  await expect
    .poll(async () => {
      const state = (await (
        await page.request.get("/mock/discord/state")
      ).json()) as { channels: { id: string; failure: string }[] };
      return state.channels.find((channel) => channel.id === "400")?.failure;
    })
    .toBe("forbidden");
  await page
    .getByRole("button", { name: "# other-updates", exact: true })
    .click();
  await expect(behavior).toHaveValue("");
  await behavior.selectOption("rate-limit");
  await expect(behavior).toBeEnabled();
  await page.getByRole("button", { name: "# read-only", exact: true }).click();
  await expect(behavior).toBeDisabled();
  await expect(
    page.getByText(
      "This fixture channel always lacks Send Messages permission.",
    ),
  ).toBeVisible();
  await page.setViewportSize({ width: 320, height: 844 });
  await page
    .getByRole("button", { name: "# server-updates", exact: true })
    .click();
  await expect(behavior).toHaveValue("forbidden");
  await behavior.selectOption("");
  await expect(behavior).toBeEnabled();
  await behavior.scrollIntoViewIfNeeded();
  await expect(behavior).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await expect(page.getByRole("alert")).toBeHidden();
});
