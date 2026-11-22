import { test, expect } from "./fixtures";
import { login } from "./screens";
import { readAuthenticatedSession } from "./auth";

for (const width of [1400, 390]) {
  test(`new token secrets receive focus and remain in view above a populated list at ${width}px`, async ({
    page,
    baseURL,
  }) => {
    await login(page);
    const session = await readAuthenticatedSession(
      await page.request.get("/api/v1/auth/session"),
    );
    for (let index = 0; index < 30; index++) {
      const response = await page.request.post("/api/v1/tokens", {
        headers: { "X-CSRF-Token": session.csrf_token, Origin: baseURL! },
        data: {
          name: `Existing automation ${index}`,
          expires_in_days: 90,
          scopes: ["*"],
        },
      });
      expect(response.status()).toBe(201);
    }
    await page
      .getByRole("button", {
        name: "Account options for Developer",
        exact: true,
      })
      .click();
    await page.getByRole("button", { name: "API tokens", exact: true }).click();
    await page.setViewportSize({ width, height: 844 });
    const drawer = page.getByRole("dialog", {
      name: "API tokens",
      exact: true,
    });
    const create = drawer.getByRole("button", {
      name: "Create token",
      exact: true,
    });
    await expect(drawer.getByRole("table").getByRole("row")).toHaveCount(31);
    await create.click();
    await drawer
      .getByRole("textbox", { name: "Name", exact: true })
      .fill("New automation");
    await drawer
      .getByRole("checkbox", {
        name: "All my current permissions",
        exact: true,
      })
      .uncheck();
    await drawer
      .getByRole("checkbox", { name: "Delete server", exact: true })
      .check();
    await expect(
      drawer.getByRole("textbox", { name: "Name", exact: true }),
    ).not.toBeInViewport();
    await create.click();
    const banner = drawer.getByRole("region", { name: "New token secret" });
    await expect(banner).toBeFocused();
    await expect(banner).toBeInViewport({ ratio: 1 });
    await expect(
      banner.getByRole("button", { name: "Copy token", exact: true }),
    ).toBeInViewport({ ratio: 1 });
    await expect(drawer.getByRole("table").getByRole("row")).toHaveCount(32);
  });
}

test("API tokens open as a list, create through a separate form, and return with a copyable secret", async ({
  page,
}) => {
  await login(page);
  await page
    .getByRole("button", { name: "Account options for Developer", exact: true })
    .click();
  await page.getByRole("button", { name: "API tokens", exact: true }).click();
  const drawer = page.getByRole("dialog", { name: "API tokens", exact: true });
  const create = drawer.getByRole("button", {
    name: "Create token",
    exact: true,
  });
  await expect(drawer.getByText("No active API tokens.")).toBeVisible();
  await expect(
    drawer.getByRole("textbox", { name: "Name", exact: true }),
  ).toHaveCount(0);

  await create.click();
  await expect(
    drawer.getByRole("textbox", { name: "Name", exact: true }),
  ).toBeFocused();
  await drawer.getByRole("button", { name: "Back", exact: true }).click();
  await expect(drawer.getByText("No active API tokens.")).toBeVisible();
  await create.click();
  await drawer
    .getByRole("textbox", { name: "Name", exact: true })
    .fill("Backup automation");
  await create.click();

  const table = drawer.getByRole("table", { name: "Active tokens" });
  const banner = drawer.getByRole("region", { name: "New token secret" });
  await expect(
    table.getByRole("row").filter({ hasText: "Backup automation" }),
  ).toBeVisible();
  await expect(banner).toBeVisible();
  await expect(
    drawer.getByRole("textbox", { name: "Name", exact: true }),
  ).toHaveCount(0);
  await expect(banner.getByRole("textbox")).toHaveCount(0);
  const secret = await banner.locator("code").textContent();
  expect(secret).toBeTruthy();
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await banner.getByRole("button", { name: "Copy token", exact: true }).click();
  await expect(
    banner.getByRole("button", { name: "Copied", exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    secret,
  );

  await create.click();
  await expect(table).toHaveCount(0);
  await expect(banner).toHaveCount(0);
  await drawer
    .getByRole("textbox", { name: "Name", exact: true })
    .fill("Audit reader");
  await drawer
    .getByRole("checkbox", { name: "All my current permissions", exact: true })
    .uncheck();
  await drawer
    .getByRole("checkbox", { name: "View audit log", exact: true })
    .check();
  await expect(
    drawer.getByRole("checkbox", { name: "View server", exact: true }),
  ).not.toBeChecked();
  await create.click();
  await expect(table.getByRole("row")).toHaveCount(3);
  await expect(
    table.getByRole("row").filter({ hasText: "Audit reader" }),
  ).toContainText("View audit log");
  await expect(banner.locator("code")).not.toHaveText(secret!);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(table).toBeVisible();
  await expect(banner).toBeVisible();
});
