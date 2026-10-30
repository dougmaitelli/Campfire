import { test, expect, screenshot } from "./fixtures";
import { login, openScreen } from "./screens";

test("server permission presets replace the selection with their complete grants", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "roles", "Roles");
  await page
    .getByRole("button", { name: "+ Create role", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Create role", exact: true });
  const preset = drawer.getByRole("combobox", {
    name: "Apply a permission preset",
    exact: true,
  });
  await preset.click();
  await expect(page.getByRole("option")).toHaveCount(5);
  expect((await page.getByRole("option").allTextContents()).sort()).toEqual(
    [
      "Choose a preset…",
      "Log viewer",
      "Basic operator",
      "Backup maintainer",
      "Server administrator",
    ].sort(),
  );
  await page.getByRole("option", { name: "Log viewer", exact: true }).click();
  async function selectedPermissions() {
    return drawer
      .locator('input[type="checkbox"]:checked')
      .evaluateAll((boxes) =>
        boxes
          .map((box) => box.closest("li")!.getAttribute("data-permission")!)
          .sort(),
      );
  }
  expect(await selectedPermissions()).toEqual([
    "server.console.read",
    "server.view",
  ]);
  await preset.click();
  await page
    .getByRole("option", { name: "Basic operator", exact: true })
    .click();
  expect(await selectedPermissions()).toEqual([
    "server.console.read",
    "server.operate",
    "server.view",
  ]);
  await preset.click();
  await page
    .getByRole("option", { name: "Backup maintainer", exact: true })
    .click();
  expect(await selectedPermissions()).toEqual([
    "server.backups.create",
    "server.backups.download",
    "server.backups.read",
    "server.backups.restore",
    "server.console.read",
    "server.operate",
    "server.view",
  ]);
  await preset.click();
  await screenshot(page, "role-permission-presets.png");
  await page
    .getByRole("option", { name: "Server administrator", exact: true })
    .click();
  const catalogResponse = await page.request.get("/api/v1/permissions");
  expect(catalogResponse.ok()).toBeTruthy();
  const catalog = (await catalogResponse.json()) as { id: string }[];
  const administratorPermissions = catalog
    .filter((item) => item.id !== "server.delete")
    .map((item) => item.id)
    .sort();
  expect(await selectedPermissions()).toEqual(administratorPermissions);
  await drawer
    .getByLabel("Name", { exact: true })
    .fill("Server administrators");
  await drawer.getByRole("button", { name: "Save role", exact: true }).click();
  await expect(drawer).toHaveCount(0);
  const response = await page.request.get("/api/v1/roles");
  const roles = (await response.json()) as {
    name: string;
    scope: string;
    permissions: string[];
  }[];
  const saved = roles.find((role) => role.name === "Server administrators");
  expect(saved?.scope).toBe("server");
  expect(saved?.permissions.slice().sort()).toEqual(administratorPermissions);
});

test("permission tree adds prerequisites and clears dependent grants without confirmation", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "roles", "Roles");
  await page
    .getByRole("button", { name: "+ Create role", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "Create role", exact: true });
  await dialog
    .getByLabel("Name", { exact: true })
    .fill("Backup restoration crew");
  await dialog
    .getByRole("combobox", { name: "Apply a permission preset", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Backup maintainer", exact: true })
    .click();
  for (const name of ["Create backups", "Download backups"]) {
    await dialog.getByRole("checkbox", { name, exact: true }).uncheck();
  }
  const restore = dialog.getByRole("checkbox", {
    name: "Restore backups",
    exact: true,
  });
  const write = dialog.getByRole("checkbox", {
    name: "Write files",
    exact: true,
  });
  const operate = dialog.getByRole("checkbox", {
    name: "Operate server",
    exact: true,
  });
  const confirmations: string[] = [];
  page.on("dialog", async (confirmation) => {
    confirmations.push(confirmation.message());
    await confirmation.accept();
  });
  await expect(dialog.getByRole("checkbox")).toHaveCount(18);
  const mainBranches = dialog.locator(
    'li[data-permission="server.view"] > ul > li',
  );
  expect(
    await mainBranches.evaluateAll((branches) =>
      branches.map((branch) => branch.getAttribute("data-permission")),
    ),
  ).toEqual(["server.console.read"]);
  const operationBranches = dialog.locator(
    'li[data-permission="server.operate"] > ul > li',
  );
  expect(
    await operationBranches.evaluateAll((branches) =>
      branches.map((branch) => branch.getAttribute("data-permission")).sort(),
    ),
  ).toEqual(
    [
      "server.console.write",
      "server.configure",
      "server.reinstall",
      "server.files.read",
      "server.backups.read",
      "server.schedules.read",
      "server.delete",
    ].sort(),
  );
  const deleteServer = dialog.getByRole("checkbox", {
    name: "Delete server",
    exact: true,
  });
  expect(
    await dialog
      .locator('li[data-permission="server.backups.read"] > ul > li')
      .evaluateAll((branches) =>
        branches.map((branch) => branch.getAttribute("data-permission")).sort(),
      ),
  ).toEqual(
    [
      "server.backups.create",
      "server.backups.restore",
      "server.backups.download",
      "server.backups.delete",
    ].sort(),
  );
  await expect(
    dialog.locator(
      'li[data-permission="server.files.read"] > ul > li[data-permission="server.files.write"] > .permission-tree-node > label > input',
    ),
  ).not.toBeChecked();
  await expect(
    dialog.locator(
      'li[data-permission="server.files.write"] > ul > li[data-permission="server.files.delete"] > .permission-tree-node > label > input',
    ),
  ).not.toBeChecked();
  await expect(restore).toHaveAccessibleDescription(
    "Restore game files from a backup",
  );
  await expect(dialog.getByText(/Also requires:/)).toHaveCount(0);
  await expect(restore).toBeChecked();
  await expect(write).not.toBeChecked();
  const reinstall = dialog.getByRole("checkbox", {
    name: "Update or reinstall",
    exact: true,
  });
  const configure = dialog.getByRole("checkbox", {
    name: "Configure server",
    exact: true,
  });
  await reinstall.check();
  await expect(configure).not.toBeChecked();
  await configure.check();
  await configure.uncheck();
  await expect(reinstall).toBeChecked();
  await reinstall.uncheck();
  await expect(
    dialog.getByRole("checkbox", { name: "Operate server", exact: true }),
  ).toBeChecked();
  await expect(
    dialog.getByRole("checkbox", { name: "Delete files", exact: true }),
  ).not.toBeChecked();
  await screenshot(page, "role-permissions.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "role-permissions-mobile.png");
  await expect(
    dialog.locator(".permission-tree-node").first(),
  ).toHaveScreenshot("permission-parent-connector-mobile.png");
  await restore.scrollIntoViewIfNeeded();
  await screenshot(page, "role-permissions-backups-mobile.png");
  await deleteServer.scrollIntoViewIfNeeded();
  await screenshot(page, "role-permissions-danger-mobile.png");
  await write.check();
  await write.uncheck();
  await expect(restore).toBeChecked();
  await operate.uncheck();
  await expect(restore).not.toBeChecked();
  for (const name of [
    "Configure server",
    "Update or reinstall",
    "Read files",
    "Write files",
    "View backups",
    "View schedules",
  ]) {
    await expect(
      dialog.getByRole("checkbox", { name, exact: true }),
    ).not.toBeChecked();
  }
  await operate.check();
  await expect(restore).not.toBeChecked();
  expect(confirmations).toEqual([]);
  await dialog.getByRole("button", { name: "Save role", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  const response = await page.request.get("/api/v1/roles");
  const roles = (await response.json()) as {
    name: string;
    permissions: string[];
  }[];
  const saved = roles.find((item) => item.name === "Backup restoration crew");
  expect(saved?.permissions).toContain("server.operate");
  expect(saved?.permissions).not.toContain("server.backups.restore");
  expect(saved?.permissions).not.toContain("server.files.write");
  await page.getByRole("button", { name: "Admin", exact: true }).click();
  const builtin = page.getByRole("dialog", { name: "View role", exact: true });
  await expect(
    builtin.getByRole("checkbox", { name: "Manage users", exact: true }),
  ).toBeChecked();
  await expect(
    builtin.getByRole("checkbox", { name: "Manage users", exact: true }),
  ).toBeDisabled();
});

test("file readers inherit operation and logs without file writing or console commands", async ({
  page,
  request,
  baseURL,
}) => {
  await login(page, "viewer");
  const auth = await request.post("/api/v1/auth/login", {
    data: { username: "admin", password: "campfire-e2e-password" },
  });
  expect(auth.ok()).toBeTruthy();
  const { csrf_token } = (await auth.json()) as { csrf_token: string };
  const headers = { "X-CSRF-Token": csrf_token, Origin: baseURL! };
  const response = await request.post("/api/v1/roles", {
    headers,
    data: {
      name: "File reader",
      scope: "server",
      permissions: ["server.view", "server.files.read"],
    },
  });
  expect(response.ok()).toBeTruthy();
  const role = (await response.json()) as { id: number };
  expect(
    (
      await request.post("/api/v1/servers/survival/members", {
        headers,
        data: { username: "viewer", role_id: role.id },
      })
    ).ok(),
  ).toBeTruthy();
  await page.goto("/#servers/survival/files");
  const tabs = page.getByRole("navigation", {
    name: "Server operations",
    exact: true,
  });
  await expect(
    tabs.getByRole("link", { name: "Files", exact: true }),
  ).toBeVisible();
  await expect(
    tabs.getByRole("link", { name: "Console", exact: true }),
  ).toBeVisible();
  await expect(
    tabs.getByRole("link", { name: "Backups", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Upload Files", exact: true }),
  ).toBeDisabled();
  await expect(page.getByRole("alert")).toHaveCount(0);
  const files = await page.evaluateHandle(() => {
    const transfer = new DataTransfer();
    transfer.items.add(new File(["read only"], "forbidden.txt"));
    return transfer;
  });
  const browser = page.getByRole("region", {
    name: "File browser",
    exact: true,
  });
  await browser.dispatchEvent("dragenter", { dataTransfer: files });
  await expect(
    page.getByText("Release to upload", { exact: true }),
  ).toHaveCount(0);
  await browser.dispatchEvent("drop", { dataTransfer: files });
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "forbidden.txt", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Delete", exact: true }),
  ).toBeDisabled();
  await screenshot(page, "file-reader.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "file-reader-mobile.png");
  await page.getByRole("button", { name: "eula.txt", exact: true }).click();
  await expect(page.getByRole("textbox")).toHaveAttribute("readonly", "");
  await expect(
    page.getByRole("button", { name: "Save file", exact: true }),
  ).toBeDisabled();
  expect(
    (await page.request.get("/api/v1/servers/survival/console")).status(),
  ).toBe(200);
  await page.goto("/#servers/survival/console");
  await expect(
    page.getByRole("textbox", { name: "Command", exact: true }),
  ).toHaveCount(0);
});
