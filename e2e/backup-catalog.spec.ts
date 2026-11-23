import { test, expect } from "./fixtures";
import { readAuthenticatedSession } from "./auth";
import type { CatalogBackup, BackupListing } from "../web/src/api/backups";
import { login } from "./screens";
import { BuiltinRole } from "../web/src/config/policies";

test("overview and backup catalog show only backups from assigned servers", async ({
  page,
}) => {
  await login(page);
  const session = await readAuthenticatedSession(
    await page.request.get("/api/v1/auth/session"),
  );
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
    .fill("Campfire-E2E-Password1!");
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
  const catalog = (await response.json()) as CatalogBackup[];
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

test("independent archives can be found by current storage Spark or archive ID", async ({
  page,
}) => {
  await login(page);
  const session = await readAuthenticatedSession(
    await page.request.get("/api/v1/auth/session"),
  );
  const listingResponse = await page.request.get(
    "/api/v1/servers/survival/backups",
  );
  expect(listingResponse.ok()).toBeTruthy();
  const listing = (await listingResponse.json()) as BackupListing;
  expect(listing.entries.length).toBeGreaterThan(0);
  const deleted = await page.request.delete("/api/v1/servers/survival", {
    headers: { "X-CSRF-Token": session.csrf_token },
  });
  expect(deleted.ok()).toBeTruthy();
  const catalog = (await (
    await page.request.get("/api/v1/backups")
  ).json()) as CatalogBackup[];
  const archived = catalog.find((item) => item.id === listing.entries[0]?.id);
  expect(archived?.server_id).toBeFalsy();
  expect(archived?.local_spark_id).toBeTruthy();
  expect(archived?.local_spark_name).toBeTruthy();
  if (!archived?.local_spark_id || !archived.local_spark_name)
    throw new Error("Missing archive storage location");
  const { id, local_spark_id: sparkID, local_spark_name: sparkName } = archived;
  await page.goto("/#backups");
  const table = page.getByRole("table");
  const linked = table.locator("tbody > .table-row-caret").first();
  const independent = table.locator("tbody > tr:not(.table-row-caret)").first();
  for (const viewport of [
    { width: 1440, height: 1000 },
    { width: 390, height: 844 },
  ]) {
    await page.setViewportSize(viewport);
    await expect(linked).toBeVisible();
    await expect(independent).toBeVisible();
    const gutter = await linked.locator("td:last-child").evaluate((cell) => {
      const style = getComputedStyle(cell);
      return {
        actual: parseFloat(style.paddingInlineEnd),
        expected: parseFloat(style.getPropertyValue("--table-row-caret-space")),
      };
    });
    expect(gutter.actual).toBe(gutter.expected);
    const linkedActions = await linked.locator(".action-group").boundingBox();
    const independentActions = await independent
      .locator(".action-group")
      .boundingBox();
    if (!linkedActions || !independentActions)
      throw new Error("Missing archive actions");
    expect(
      Math.abs(
        linkedActions.x +
          linkedActions.width -
          independentActions.x -
          independentActions.width,
      ),
    ).toBeLessThan(1);
    expect(
      await independent
        .locator("td:last-child")
        .evaluate((cell) => getComputedStyle(cell, "::after").content),
    ).toBe("none");
    await independent.locator("td").first().click();
    await expect(page).toHaveURL(/#backups$/);
    await expect(
      page
        .locator(".page-header .page-actions")
        .getByRole("button", { name: "Upload backup", exact: true }),
    ).toBeVisible();
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  const search = page.getByRole("searchbox");
  await search.fill(`spark:${sparkID}`);
  await expect(page.getByRole("table").locator("tbody tr")).toHaveCount(
    catalog.filter((item) => item.local_spark_id === sparkID).length,
  );
  await search.fill(id);
  await expect(page.getByRole("table").locator("tbody tr")).toHaveCount(1);
  await expect(page.getByRole("table").locator("tbody")).toContainText(
    "No server",
  );
  await expect(page.getByRole("table").locator("tbody")).toContainText(
    `Spark: ${sparkName}`,
  );
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
  await expect(
    page.getByRole("button", { name: "Upload backup", exact: true }),
  ).toHaveCount(0);
});
