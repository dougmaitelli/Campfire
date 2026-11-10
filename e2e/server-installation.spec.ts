import { test, expect, screenshot } from "./fixtures";
import { login, mockServerStatus } from "./screens";
import {
  OperationState,
  RuntimeErrorCode,
  RuntimeState,
  ServerAction,
} from "../web/src/contracts/domains";

test("first installation starts from an empty state and keeps failed installer output accessible", async ({
  page,
}) => {
  await login(page);
  const initial = {
    server_id: "survival",
    state: RuntimeState.NotInstalled,
    ready: false,
    busy: false,
    disk_quota_enforced: false,
  };
  let status: Record<string, unknown> = initial;
  await mockServerStatus(page, "survival", () => status);
  let release = () => {};
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  let attempts = 0;
  await page.route("**/api/v1/servers/survival/actions", async (route) => {
    expect(route.request().postDataJSON()).toMatchObject({
      action: ServerAction.Install,
    });
    attempts++;
    if (attempts === 1) {
      status = {
        ...initial,
        state: RuntimeState.Installing,
        busy: true,
        operation: ServerAction.Install,
        operation_state: OperationState.Running,
      };
      await pending;
      status = {
        ...initial,
        operation: ServerAction.Install,
        operation_state: OperationState.Failed,
        error_code: RuntimeErrorCode.InstallerFailed,
        error: "Installer script failed.",
      };
    } else {
      status = {
        ...initial,
        state: RuntimeState.Stopped,
        operation: ServerAction.Install,
        operation_state: OperationState.Succeeded,
      };
    }
    await route.fulfill({ json: status });
  });

  await page.goto("/#servers/survival/files");
  await expect(
    page.getByRole("heading", { name: "Ready to install", exact: true }),
  ).toBeVisible();
  const tabs = page.getByRole("navigation", {
    name: "Server operations",
    exact: true,
  });
  const consoleOutput = page.getByRole("region", {
    name: "Console output",
    exact: true,
  });
  await expect(tabs).toBeHidden();
  await expect(consoleOutput).toBeHidden();
  await screenshot(page, "server-install-empty.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "server-install-empty-mobile.png");
  await page.setViewportSize({ width: 1400, height: 900 });
  try {
    await page.getByRole("button", { name: "Install", exact: true }).click();
    await expect(page).toHaveURL(/#servers\/survival\/console$/);
    await expect(tabs).toBeVisible();
    await expect(consoleOutput).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Ready to install", exact: true }),
    ).toBeHidden();
  } finally {
    release();
  }
  const retry = page.getByRole("button", {
    name: "Retry installation",
    exact: true,
  });
  await expect(retry).toBeEnabled();
  await expect(
    page.getByText("Installation failed.", { exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(consoleOutput).toBeVisible();
  await expect(retry).toBeEnabled();
  await retry.click();
  await expect(retry).toBeHidden();
  await expect(consoleOutput).toBeVisible();
  expect(attempts).toBe(2);
});

test("view-only users can read Activity before installation without an install action", async ({
  page,
}) => {
  await login(page);
  await mockServerStatus(page, "survival", () => ({
    server_id: "survival",
    state: RuntimeState.NotInstalled,
    ready: false,
    busy: false,
    disk_quota_enforced: false,
  }));
  await page.route("**/api/v1/servers/survival", async (route) => {
    const response = await route.fetch();
    const server = await response.json();
    await route.fulfill({
      response,
      json: { ...server, permissions: ["server.view"] },
    });
  });
  await page.goto("/#servers/survival/console");
  await expect(
    page.getByRole("heading", { name: "Activity", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("region", { name: "Activity", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Install", exact: true }),
  ).toBeHidden();
});
