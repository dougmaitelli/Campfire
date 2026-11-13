import type { Page } from "@playwright/test";
import { test, expect } from "./fixtures";
import { login, mockServerStatus } from "./screens";
import { RuntimeState, ServerAction } from "../web/src/contracts/domains";

async function openProperties(page: Page) {
  await page.goto("/#servers/survival/files");
  const state = page.locator(".page-header .server-state");
  await expect(state).toBeVisible();
  await expect(state).not.toContainText("Unavailable");
  await page
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  return page.getByRole("textbox", {
    name: "/server.properties",
    exact: true,
  });
}

for (const [action, state] of [
  [ServerAction.Start, RuntimeState.Starting],
  [ServerAction.Stop, RuntimeState.Stopping],
  [ServerAction.Restart, RuntimeState.Restarting],
  [ServerAction.Kill, RuntimeState.Stopping],
] as const) {
  test(`files can be browsed and edited during ${action}`, async ({ page }) => {
    await login(page);
    await mockServerStatus(page, "survival", () => ({
      server_id: "survival",
      state,
      ready: false,
      busy: true,
      file_system_busy: false,
      disk_quota_enforced: true,
      operation: action,
      operation_state: "running",
    }));
    const editor = await openProperties(page);
    await expect(editor).toBeEditable();
    await editor.fill("motd=Community server maintenance\n");
    await page.getByRole("button", { name: "Save file", exact: true }).click();
    await page.reload();
    await page
      .getByRole("button", { name: "server.properties", exact: true })
      .click();
    await expect(editor).toHaveValue("motd=Community server maintenance\n");
  });
}

for (const [action, state] of [
  [ServerAction.Install, RuntimeState.Installing],
  [ServerAction.Update, RuntimeState.Updating],
  [ServerAction.Reinstall, RuntimeState.Reinstalling],
  [ServerAction.Restore, RuntimeState.Stopped],
  [ServerAction.Delete, RuntimeState.Deleting],
  [ServerAction.Transfer, RuntimeState.Stopped],
  [ServerAction.Backup, RuntimeState.Running],
  [undefined, RuntimeState.Running],
] as const) {
  test(`file editing stays blocked during ${action ?? "an unspecified busy operation"}`, async ({
    page,
  }) => {
    await login(page);
    await mockServerStatus(page, "survival", () => ({
      server_id: "survival",
      state,
      ready: state === RuntimeState.Running,
      busy: true,
      file_system_busy: true,
      disk_quota_enforced: true,
      ...(action ? { operation: action } : {}),
      operation_state: "running",
    }));
    await expect(await openProperties(page)).not.toBeEditable();
  });
}

for (const busy of [false, true]) {
  test(`older Sparks fall back to busy=${busy} for file editing`, async ({
    page,
  }) => {
    await login(page);
    await mockServerStatus(page, "survival", () => ({
      server_id: "survival",
      state: busy ? RuntimeState.Restarting : RuntimeState.Stopped,
      ready: false,
      busy,
      disk_quota_enforced: true,
    }));
    const editor = await openProperties(page);
    if (busy) await expect(editor).not.toBeEditable();
    else await expect(editor).toBeEditable();
  });
}
