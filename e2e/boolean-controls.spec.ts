import { test, expect } from "./fixtures";
import { login, openScreen, openServerEditor } from "./screens";

test("user Enabled switches preserve disabled values and readonly table state", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "users", "Users");
  const row = page.getByRole("row").filter({ hasText: "Taylor Brooks" });
  const readonly = row.getByRole("switch", { name: "Enabled", exact: true });
  await expect(readonly).not.toBeChecked();
  await expect(readonly).toBeDisabled();
  for (const enabled of [true, false]) {
    await row
      .getByRole("button", { name: "Edit user Taylor Brooks", exact: true })
      .click();
    const drawer = page.getByRole("dialog", { name: "Edit user", exact: true });
    await drawer
      .getByRole("switch", { name: "Enabled", exact: true })
      .setChecked(enabled);
    const saved = page.waitForResponse(
      (response) =>
        /\/api\/v1\/users\/\d+$/.test(response.url()) &&
        response.request().method() === "PUT",
    );
    await drawer
      .getByRole("button", { name: "Save user", exact: true })
      .click();
    expect(await (await saved).json()).toMatchObject({ disabled: !enabled });
    await expect(drawer).toHaveCount(0);
    await page.reload();
    await expect(readonly).toBeChecked({ checked: enabled });
  }
});

test("Spark Enabled switches stay separate from connection status", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "sparks", "Sparks");
  const row = page.getByRole("row").filter({ hasText: "mock-offline" });
  for (const enabled of [false, true]) {
    await row
      .getByRole("button", { name: "View details for Offline", exact: true })
      .click();
    const details = page.getByRole("dialog");
    await expect(
      details.getByRole("switch", { name: "Enabled", exact: true }),
    ).toBeDisabled();
    await details
      .getByRole("button", { name: "Edit Spark", exact: true })
      .click();
    const drawer = page.getByRole("dialog", {
      name: "Edit Spark",
      exact: true,
    });
    await drawer
      .getByRole("switch", { name: "Enabled", exact: true })
      .setChecked(enabled);
    await drawer.getByRole("button", { name: "Review", exact: true }).click();
    const review = drawer.getByRole("switch", { name: "Enabled", exact: true });
    await expect(review).toBeChecked({ checked: enabled });
    await expect(review).toBeDisabled();
    await drawer
      .getByRole("button", { name: "Save changes", exact: true })
      .click();
    await expect(drawer).toHaveCount(0);
    await page.reload();
    await expect(
      row.getByRole("switch", { name: "Enabled", exact: true }),
    ).toBeChecked({ checked: enabled });
    await expect(row).toContainText("Disconnected");
    const response = await page.request.get("/api/v1/sparks/mock-offline");
    expect(await response.json()).toMatchObject({ disabled: !enabled });
  }
});

test("game integration uses an editable switch and a readonly review switch", async ({
  page,
}) => {
  await login(page);
  const drawer = await openServerEditor(page);
  const integration = drawer.getByRole("switch", {
    name: "Enable game integration",
    exact: true,
  });
  await expect(integration).toBeChecked();
  await drawer
    .getByRole("switch", { name: "Enable game integration", exact: true })
    .uncheck();
  await drawer.getByRole("button", { name: "Review", exact: true }).click();
  await expect(
    drawer.getByRole("switch", {
      name: "Enable game integration",
      exact: true,
    }),
  ).not.toBeChecked();
  await drawer
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(drawer).toHaveCount(0);
  const response = await page.request.get("/api/v1/servers/creative");
  expect(await response.json()).toMatchObject({ game_integration: false });
});

for (const scenario of [
  { available: false, storage: "local" },
  { available: false, storage: "s3" },
  { available: true, storage: "local" },
]) {
  test(`S3 sync availability ${scenario.available} with ${scenario.storage} storage`, async ({
    page,
  }) => {
    await login(page);
    await page.route(
      "**/api/v1/servers/creative/backup-policy",
      async (route) => {
        const response = await route.fetch();
        const body = await response.json();
        await route.fulfill({
          response,
          json: {
            ...body,
            s3_available: scenario.available,
            policy: { ...body.policy, storage: scenario.storage },
          },
        });
      },
    );
    const drawer = await openServerEditor(page);
    await drawer.getByRole("button", { name: "Backups", exact: true }).click();
    const sync = drawer.getByRole("switch", {
      name: "Background S3 sync",
      exact: true,
    });
    await expect(sync).toBeChecked({ checked: scenario.storage === "s3" });
    if (scenario.available) {
      await sync.check();
      await expect(sync).toBeChecked();
      await sync.press("Space");
      await expect(sync).not.toBeChecked();
    } else if (scenario.storage === "s3") {
      await expect(sync).toBeEnabled();
      await sync.uncheck();
      await expect(sync).toBeDisabled();
    } else {
      await expect(sync).toBeDisabled();
    }
  });
}
