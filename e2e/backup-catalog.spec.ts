import { test, expect } from "./fixtures";
import { login } from "./screens";
import { BuiltinRole } from "../web/src/config/policies";

test("overview and backup catalog show only backups from assigned servers", async ({
  page,
}) => {
  await login(page);
  const session = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  const grant = await page.request.post("/api/v1/servers/modded/members", {
    headers: { "X-CSRF-Token": session.csrf_token },
    data: { username: "jordan", role_id: BuiltinRole.Operator },
  });
  expect(grant.ok()).toBeTruthy();
  await page.context().clearCookies();
  await page.goto("/");
  await page.getByLabel("Username", { exact: true }).fill("jordan");
  await page
    .getByLabel("Password", { exact: true })
    .fill("campfire-e2e-password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  const recent = page.getByRole("table", {
    name: "Recent backups",
    exact: true,
  });
  await expect(recent.locator("tbody tr")).toHaveCount(2);
  await expect(
    recent.getByRole("link", { name: "Modded", exact: true }),
  ).toHaveCount(2);
  const response = await page.request.get("/api/v1/backups");
  expect(response.ok()).toBeTruthy();
  const catalog = (await response.json()) as { server_id: string }[];
  expect(catalog).toHaveLength(2);
  expect(catalog.every((entry) => entry.server_id === "modded")).toBe(true);
  await page
    .getByRole("link", { name: "View all backups", exact: true })
    .click();
  await expect(page).toHaveURL(/#backups$/);
  await expect(
    page.getByRole("heading", { name: "Backups", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("table").locator("tbody tr")).toHaveCount(2);
  await expect(page.getByRole("link", { name: /^Download / })).toHaveCount(2);
});

test("server viewing without backup viewing returns an empty catalog", async ({
  page,
}) => {
  await login(page, "observer");
  const response = await page.request.get("/api/v1/backups");
  expect(response.ok()).toBeTruthy();
  expect(await response.json()).toEqual([]);
  await page
    .getByRole("link", { name: "View all backups", exact: true })
    .click();
  await expect(page).toHaveURL(/#backups$/);
  await expect(
    page.getByRole("heading", { name: "No backups yet", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: /^Download / })).toHaveCount(0);
});
