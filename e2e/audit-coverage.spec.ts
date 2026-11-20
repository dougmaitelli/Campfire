import { test, expect } from "./fixtures";
import { login } from "./screens";
import { readAuthenticatedSession } from "./auth";
import type { AuditEvent } from "../web/src/api/activity";
import type { CatalogBackup } from "../web/src/api/backups";
import type { Page as Paginated } from "../web/src/api/pagination";
import { AuditOutcome, FileOperation } from "../web/src/contracts/domains";

test("backup downloads appear in Audit without routine request noise", async ({
  page,
}) => {
  await login(page);
  const { csrf_token } = await readAuthenticatedSession(
    await page.request.get("/api/v1/auth/session"),
  );
  const catalogResponse = await page.request.get("/api/v1/backups");
  expect(catalogResponse.ok()).toBeTruthy();
  const catalog = (await catalogResponse.json()) as CatalogBackup[];
  const backup = catalog.find((item) => item.server_id === "survival");
  if (!backup) throw new Error("Missing Survival backup");
  const download = await page.request.get(
    `/api/v1/backups/${encodeURIComponent(backup.id)}`,
  );
  expect(download.ok()).toBeTruthy();
  expect((await download.body()).length).toBeGreaterThan(0);
  const audit = async (action: string) => {
    const response = await page.request.get(
      `/api/v1/audit?action=${encodeURIComponent(action)}`,
    );
    expect(response.ok()).toBeTruthy();
    return (await response.json()) as Paginated<AuditEvent>;
  };
  await expect
    .poll(async () =>
      (await audit("server.backup.download")).items.some(
        (item) =>
          item.target === `backup:${backup.id}` &&
          item.outcome === AuditOutcome.Succeeded,
      ),
    )
    .toBe(true);
  for (const request of [
    {
      path: "/api/v1/notifications/read-all",
      action: "notification.read",
      data: {},
    },
    {
      path: "/api/v1/schedules/preview",
      action: "schedule.preview",
      data: { cron_expression: "0 5 * * *", time_zone: "UTC" },
    },
    {
      path: "/api/v1/servers/survival/files",
      action: "server.files.list",
      data: { operation: FileOperation.List, path: "." },
    },
  ]) {
    const before = (await audit(request.action)).pagination.total_items;
    const response = await page.request.post(request.path, {
      headers: { "X-CSRF-Token": csrf_token },
      data: request.data,
    });
    expect(response.ok()).toBeTruthy();
    expect((await audit(request.action)).pagination.total_items).toBe(before);
  }
  await page.goto("/#audit?action=server.backup.download");
  await expect(
    page.getByRole("heading", { name: "Audit", exact: true }),
  ).toBeVisible();
  const row = page.getByRole("row").filter({ hasText: `backup:${backup.id}` });
  await expect(row).toContainText("server.backup.download");
  await expect(row).toContainText("Succeeded");
});
