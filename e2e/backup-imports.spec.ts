import { test, expect, screenshot } from "./fixtures";
import { login, openScreen } from "./screens";
import type { Page } from "@playwright/test";

async function portableBackup(page: Page, mode: "full" | "savedata") {
  const session = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  const created = await page.request.post("/api/v1/servers/survival/actions", {
    data: { action: "backup", backup_mode: mode },
    headers: {
      "X-CSRF-Token": session.csrf_token,
      Origin: "http://127.0.0.1:8099",
    },
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  const status = (await created.json()) as { operation_id: string };
  const name = `${mode}-${status.operation_id}.tar.gz`;
  const response = await page.request.get(
    `/api/v1/servers/survival/backups/${name}`,
  );
  expect(response.ok(), await response.text()).toBeTruthy();
  return {
    name: "recovery.tar.gz",
    mimeType: "application/gzip",
    buffer: await response.body(),
  };
}

test("portable backups upload into an existing catalog before a separate restore", async ({
  page,
}) => {
  await login(page);
  const archive = await portableBackup(page, "savedata");
  await page.goto("/#servers/creative/backups");
  await page
    .getByRole("button", { name: "Upload backup", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Upload backup",
    exact: true,
  });
  await drawer
    .getByLabel("Backup archive", { exact: true })
    .setInputFiles(archive);
  const upload = drawer.getByRole("button", {
    name: "Upload backup",
    exact: true,
  });
  await expect(upload).toBeEnabled();
  await expect(
    drawer.getByText("Save data only", { exact: true }),
  ).toBeVisible();
  await screenshot(page, "backup-import.png", drawer);
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "backup-import-mobile.png", drawer);
  await upload.click();
  await expect(drawer).toHaveCount(0);
  await expect(
    page.getByRole("row").filter({ hasText: "Save data only" }),
  ).toHaveCount(1);
  const catalogResponse = await page.request.get("/api/v1/backups");
  expect(catalogResponse.ok()).toBeTruthy();
  const catalog = (await catalogResponse.json()) as {
    id: string;
    server_id?: string;
    portable: boolean;
    mode: string;
  }[];
  expect(catalog.every((item) => Boolean(item.server_id))).toBe(true);
  const imported = catalog.filter(
    (item) =>
      item.server_id === "creative" &&
      item.portable &&
      item.mode === "savedata",
  );
  expect(imported).toHaveLength(1);
  expect(imported[0]?.id).toBeTruthy();
  const status = await page.request.get("/api/v1/servers/creative/status");
  expect(await status.json()).toMatchObject({ state: "stopped" });
});

for (const mode of ["full", "savedata"] as const) {
  test(`create from a ${mode} backup restores a new stopped server`, async ({
    page,
  }) => {
    await login(page);
    const archive = await portableBackup(page, mode);
    await openScreen(page, "servers", "Servers");
    await page.getByRole("button", { name: "Actions", exact: true }).click();
    if (mode === "full") {
      await page.setViewportSize({ width: 390, height: 844 });
      await expect(
        page.getByRole("menuitem", { name: "Create from backup", exact: true }),
      ).toBeVisible();
      await page.setViewportSize({ width: 1400, height: 900 });
    }
    await page
      .getByRole("menuitem", { name: "Create from backup", exact: true })
      .click();
    const upload = page.getByRole("dialog", {
      name: "Create from backup",
      exact: true,
    });
    await upload
      .getByLabel("Backup archive", { exact: true })
      .setInputFiles(archive);
    await expect(
      upload.getByRole("button", { name: "Continue", exact: true }),
    ).toBeEnabled();
    await upload.getByRole("button", { name: "Continue", exact: true }).click();
    const drawer = page.getByRole("dialog", {
      name: "Create server",
      exact: true,
    });
    await drawer
      .getByLabel("Server name", { exact: true })
      .fill(`Recovered ${mode} server`);
    if (mode === "full") {
      await screenshot(page, "create-from-backup.png", drawer);
      await page.setViewportSize({ width: 390, height: 844 });
      await screenshot(page, "create-from-backup-mobile.png", drawer);
    }
    for (const name of [
      "Next: Variables",
      "Next: Resources & ports",
      "Next: Backups",
      "Next: Access",
      "Next: Review",
    ]) {
      await drawer.getByRole("button", { name, exact: true }).click();
    }
    const created = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/servers") &&
        response.request().method() === "POST",
    );
    await drawer
      .getByRole("button", { name: "Create server", exact: true })
      .click();
    const response = await created;
    expect(response.ok()).toBeTruthy();
    const server = (await response.json()) as {
      id: string;
      spark_id: string;
      owner_user_id: number;
      permissions: string[];
    };
    await expect(drawer).toHaveCount(0);
    const status = await page.request.get(
      `/api/v1/servers/${server.id}/status`,
    );
    expect(await status.json()).toMatchObject({ state: "stopped" });
    const files = await page.request.get(
      `/api/v1/servers/${server.id}/files?path=/world/`,
    );
    expect(files.ok()).toBeTruthy();
    expect(
      ((await files.json()) as { entries: { name: string }[] }).entries.some(
        (entry) => entry.name === "level.dat",
      ),
    ).toBeTruthy();
  });
}

test("the backup library keeps completed uploads and creates a server without consuming its source", async ({
  page,
}) => {
  await login(page);
  const archive = await portableBackup(page, "full");
  // Only normalize the generated filename for the mobile snapshot. Archive IDs,
  // stored bytes, permissions, and all mutation requests still use the real API.
  await page.route("**/api/v1/backups", async (route) => {
    const response = await route.fetch();
    const entries = (await response.json()) as {
      server_id?: string;
      display_name: string;
      name: string;
    }[];
    await route.fulfill({
      response,
      json: entries.map((entry) =>
        !entry.server_id && entry.display_name === "recovery.tar.gz"
          ? { ...entry, name: "full-ABCDEFGHIJKLMNOPQRSTUVWXY2.tar.gz" }
          : entry,
      ),
    });
  });
  await openScreen(page, "backups", "Backups");
  await page
    .getByRole("button", { name: "Upload backup", exact: true })
    .click();
  const upload = page.getByRole("dialog", {
    name: "Upload backup",
    exact: true,
  });
  await upload
    .getByLabel("Backup archive", { exact: true })
    .setInputFiles(archive);
  await expect(
    upload.getByRole("button", { name: "Done", exact: true }),
  ).toBeEnabled();
  await screenshot(page, "backup-library-upload.png", upload);
  const viewport = page.viewportSize()!;
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "backup-library-upload-mobile.png", upload);
  await page.setViewportSize(viewport);
  // Cancelling after validation closes the session, not the durable archive.
  await upload.getByRole("button", { name: "Cancel", exact: true }).click();
  const row = page.getByRole("row").filter({ hasText: "recovery.tar.gz" });
  await expect(row).toContainText("No server");
  await page.getByRole("searchbox").fill("recovery.tar.gz");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "backup-library-mobile.png");
  await expect(
    row.getByRole("button", {
      name: "Backup actions for recovery.tar.gz",
      exact: true,
    }),
  ).toBeInViewport({ ratio: 1 });
  await row
    .getByRole("button", {
      name: "Backup actions for recovery.tar.gz",
      exact: true,
    })
    .click();
  await expect(
    page.getByRole("menuitem", { name: "Create from backup", exact: true }),
  ).toBeInViewport({ ratio: 1 });
  await page.keyboard.press("Escape");
  await page.setViewportSize(viewport);
  await page.unroute("**/api/v1/backups");
  await page.getByRole("searchbox").clear();
  const download = row.getByRole("link", {
    name: "Download recovery.tar.gz",
    exact: true,
  });
  const href = await download.getAttribute("href");
  expect(href).toMatch(/^\/api\/v1\/backups\/[^/]+$/);
  const before = await page.request.get(href!);
  expect(before.ok()).toBeTruthy();
  expect(await before.body()).toEqual(archive.buffer);
  await row
    .getByRole("button", {
      name: "Backup actions for recovery.tar.gz",
      exact: true,
    })
    .click();
  await page
    .getByRole("menuitem", { name: "Create from backup", exact: true })
    .click();
  const wizard = page.getByRole("dialog", {
    name: "Create server",
    exact: true,
  });
  await wizard
    .getByLabel("Server name", { exact: true })
    .fill("Library recovered server");
  for (const name of [
    "Next: Variables",
    "Next: Resources & ports",
    "Next: Backups",
    "Next: Access",
    "Next: Review",
  ]) {
    await wizard.getByRole("button", { name, exact: true }).click();
  }
  const created = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/servers") &&
      response.request().method() === "POST",
  );
  await wizard
    .getByRole("button", { name: "Create server", exact: true })
    .click();
  expect((await created).ok()).toBeTruthy();
  await expect(wizard).toHaveCount(0);
  await expect(row).toContainText("No server");
  const after = await page.request.get(href!);
  expect(after.ok()).toBeTruthy();
  expect(await after.body()).toEqual(archive.buffer);
  await expect(
    row.getByRole("button", {
      name: "Delete backup recovery.tar.gz",
      exact: true,
    }),
  ).toHaveClass(/button-danger/);
  await row
    .getByRole("button", { name: "Delete backup recovery.tar.gz", exact: true })
    .click();
  const confirmation = page.getByRole("dialog", {
    name: "Permanently delete backup?",
    exact: true,
  });
  await confirmation
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await expect(row).toBeVisible();
  await row
    .getByRole("button", { name: "Delete backup recovery.tar.gz", exact: true })
    .click();
  await confirmation
    .getByRole("button", { name: "Delete", exact: true })
    .click();
  await expect(row).toHaveCount(0);
  expect((await page.request.get(href!)).status()).toBe(404);
});

test("invalid backup uploads can be replaced and cancelled", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "servers", "Servers");
  await page.getByRole("button", { name: "Actions", exact: true }).click();
  await page
    .getByRole("menuitem", { name: "Create from backup", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Create from backup",
    exact: true,
  });
  await drawer.getByLabel("Backup archive", { exact: true }).setInputFiles({
    name: "invalid.tar.gz",
    mimeType: "application/gzip",
    buffer: Buffer.from("not a backup"),
  });
  await expect(drawer.getByRole("alert")).toContainText(
    "portable .tar.gz backup",
  );
  await expect(
    drawer.getByRole("button", { name: "Continue", exact: true }),
  ).toBeDisabled();
  await drawer.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(drawer).toHaveCount(0);
});
