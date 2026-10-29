import { test, expect, screenshot } from "./fixtures";
import { login, openScreen } from "./screens";

test("role presets add dependencies and removing file writing removes restore", async ({
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
    .getByRole("option", { name: "Backup restorer", exact: true })
    .click();
  const restore = dialog.getByRole("checkbox", { name: /^Restore game files/ });
  const write = dialog.getByRole("checkbox", { name: /^Create, upload, edit/ });
  await expect(restore).toBeChecked();
  await expect(write).toBeChecked();
  await expect(
    dialog.getByRole("checkbox", { name: /^Install, start/ }),
  ).toBeChecked();
  await expect(
    dialog.getByRole("checkbox", { name: /^Delete files/ }),
  ).not.toBeChecked();
  await screenshot(page, "role-permissions.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "role-permissions-mobile.png");
  page.once("dialog", (confirmation) => confirmation.accept());
  await write.uncheck();
  await expect(restore).not.toBeChecked();
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

test("file readers browse and open files without console or write controls", async ({
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
  ).toHaveCount(0);
  await expect(
    tabs.getByRole("link", { name: "Backups", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Upload Files", exact: true }),
  ).toBeDisabled();
  await expect(page.getByRole("alert")).toHaveCount(0);
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
  ).toBe(403);
});
