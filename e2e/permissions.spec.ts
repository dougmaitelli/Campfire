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
  const suggestedName = drawer.getByLabel("Name", { exact: true });
  await expect(suggestedName).toHaveValue("Server viewer");
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
  await expect(suggestedName).toHaveValue("Log viewer");
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
  await expect(suggestedName).toHaveValue("Basic operator");
  expect(await selectedPermissions()).toEqual([
    "server.console.read",
    "server.operate",
    "server.view",
  ]);
  await preset.click();
  await page
    .getByRole("option", { name: "Backup maintainer", exact: true })
    .click();
  await expect(suggestedName).toHaveValue("Backup maintainer");
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
  await screenshot(page, "role-permission-presets.png", drawer);
  await page
    .getByRole("option", { name: "Server administrator", exact: true })
    .click();
  await expect(suggestedName).toHaveValue("Server administrator");
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
  for (const name of ["Create, rename, and pin backups", "Download backups"]) {
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
  const deleteServer = dialog.getByRole("checkbox", {
    name: "Delete server",
    exact: true,
  });
  await expect(deleteServer).not.toBeChecked();
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
  await screenshot(page, "role-permissions.png", dialog);
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "role-permissions-mobile.png", dialog);
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
  for (const name of ["View server", "Read console"]) {
    await expect(
      dialog.getByRole("checkbox", { name, exact: true }),
    ).toBeChecked();
  }
  const deleteFiles = dialog.getByRole("checkbox", {
    name: "Delete files",
    exact: true,
  });
  await deleteFiles.check();
  await expect(write).toBeChecked();
  await expect(
    dialog.getByRole("checkbox", { name: "Read files", exact: true }),
  ).toBeChecked();
  await write.uncheck();
  await expect(deleteFiles).not.toBeChecked();
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

test("file access hides unauthorized writing and deletion while preserving reading and operation", async ({
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
  for (const name of [
    "Upload Files",
    "Create file",
    "Create folder",
    "Rename",
    "Permissions",
    "Move",
    "Archive",
    "Delete",
  ])
    await expect(page.getByRole("button", { name, exact: true })).toHaveCount(
      0,
    );
  await expect(page.getByRole("button", { name: /^Extract / })).toHaveCount(0);
  await expect(page.getByRole("checkbox")).toHaveCount(0);
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
  await page.setViewportSize({ width: 1400, height: 1200 });
  await screenshot(page, "file-reader.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "file-reader-mobile.png");
  await page.getByRole("button", { name: "eula.txt", exact: true }).click();
  await expect(page.getByRole("textbox")).toHaveAttribute("readonly", "");
  await expect(
    page.getByRole("button", { name: "Save file", exact: true }),
  ).toHaveCount(0);
  expect(
    (await page.request.get("/api/v1/servers/survival/console")).status(),
  ).toBe(200);
  await page.goto("/#servers/survival/console");
  await expect(
    page.getByRole("textbox", { name: "Command", exact: true }),
  ).toHaveCount(0);
  expect(
    (
      await request.put(`/api/v1/roles/${role.id}`, {
        headers,
        data: {
          name: "File reader",
          scope: "server",
          permissions: [
            "server.view",
            "server.files.read",
            "server.files.write",
          ],
        },
      })
    ).ok(),
  ).toBeTruthy();
  await page.goto("/#servers/survival/files");
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Create file", exact: true }),
  ).toBeEnabled();
  const rename = page.getByRole("button", { name: "Rename", exact: true });
  await expect(rename).toBeDisabled();
  await page
    .getByRole("checkbox", { name: "Select eula.txt", exact: true })
    .check();
  await expect(rename).toBeEnabled();
  await expect(
    page.getByRole("button", { name: "Delete", exact: true }),
  ).toHaveCount(0);
});
