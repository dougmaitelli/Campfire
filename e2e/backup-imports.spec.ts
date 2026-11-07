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
