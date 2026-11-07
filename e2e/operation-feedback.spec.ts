import type { Page } from "@playwright/test";
import { test, expect } from "./fixtures";
import { login } from "./screens";
import {
  BackupMode,
  OperationState,
  ServerAction,
} from "../web/src/contracts/domains";

type OperationStatus = {
  operation_id: string;
  operation_state: OperationState;
};

async function completeAction(
  page: Page,
  action: ServerAction,
  backup?: string,
) {
  const session = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  const response = await page.request.post("/api/v1/servers/survival/actions", {
    data: {
      action,
      backup,
      backup_mode: action === ServerAction.Backup ? BackupMode.Full : undefined,
    },
    headers: {
      "X-CSRF-Token": session.csrf_token,
      Origin: "http://127.0.0.1:8099",
    },
  });
  expect(response.ok(), await response.text()).toBeTruthy();
  const result = (await response.json()) as OperationStatus;
  expect(result.operation_id).toBeTruthy();
  await expect
    .poll(async () => {
      const response = await page.request.get(
        "/api/v1/servers/survival/status",
      );
      const status = (await response.json()) as OperationStatus;
      return { id: status.operation_id, state: status.operation_state };
    })
    .toEqual({ id: result.operation_id, state: OperationState.Succeeded });
  return result.operation_id;
}

for (const action of [ServerAction.Backup, ServerAction.Restore]) {
  test(`${action} confirmations only appear for actions started in the current Backups tab`, async ({
    page,
  }) => {
    await login(page);
    const backupID = await completeAction(page, ServerAction.Backup);
    const archive = `full-${backupID}.tar.gz`;
    if (action === ServerAction.Restore) {
      await completeAction(page, ServerAction.Stop);
      await completeAction(page, action, archive);
    }
    await page.goto("/#servers/survival/backups");
    const backups = page.getByRole("table", {
      name: "Server backups",
      exact: true,
    });
    await expect(
      backups.getByRole("row").filter({ hasText: archive }),
    ).toBeVisible();
    const message = page.getByText(
      action === ServerAction.Backup
        ? "Backup created."
        : "Backup restored. The server is stopped.",
      { exact: true },
    );
    await expect(message).toHaveCount(0);
    if (action === ServerAction.Backup) {
      await page
        .getByRole("button", { name: "Create backup", exact: true })
        .click();
    } else {
      await page
        .getByRole("row")
        .filter({ hasText: archive })
        .getByRole("button", { name: "Restore", exact: true })
        .click();
      await page
        .getByRole("dialog", {
          name: "Restore backup for Survival?",
          exact: true,
        })
        .getByRole("button", { name: "Restore", exact: true })
        .click();
    }
    await expect(message).toBeVisible();
    const tabs = page.getByRole("navigation", {
      name: "Server operations",
      exact: true,
    });
    await tabs.getByRole("link", { name: "Console", exact: true }).click();
    await expect(
      page.getByRole("region", { name: "Console output", exact: true }),
    ).toBeVisible();
    await tabs.getByRole("link", { name: "Backups", exact: true }).click();
    await expect(backups).toBeVisible();
    await expect(message).toHaveCount(0);
    await page.reload();
    await expect(backups).toBeVisible();
    await expect(message).toHaveCount(0);
  });
}

test("console lifecycle confirmations are limited to the current page session", async ({
  page,
}) => {
  await login(page);
  await completeAction(page, ServerAction.Stop);
  await page.goto("/#servers/survival/console");
  await expect(
    page.getByRole("button", { name: "Start", exact: true }),
  ).toBeEnabled();
  await expect(
    page.getByText("Server stop completed.", { exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Start", exact: true }).click();
  const message = page.getByText("Server start completed.", { exact: true });
  await expect(message).toBeVisible();
  await page
    .getByRole("navigation", { name: "Main navigation", exact: true })
    .getByRole("link", { name: "Servers", exact: true })
    .click();
  await page
    .getByRole("link", { name: "Open console for Survival", exact: true })
    .click();
  await expect(page.getByText("Live", { exact: true })).toBeVisible();
  await expect(message).toHaveCount(0);
});
