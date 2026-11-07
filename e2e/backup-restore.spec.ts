import { test, expect, screenshot } from "./fixtures";
import { login, openServerTab } from "./screens";

for (const access of ["read", "create", "restore"] as const) {
  test(`backup ${access} access only exposes authorized actions`, async ({
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
    const roleResponse = await request.post("/api/v1/roles", {
      headers,
      data: {
        name: `Backup ${access} access`,
        scope: "server",
        permissions: [
          "server.view",
          "server.backups.read",
          "server.backups.download",
          ...(access === "read" ? [] : [`server.backups.${access}`]),
        ],
      },
    });
    expect(roleResponse.ok()).toBeTruthy();
    const role = (await roleResponse.json()) as { id: number };
    expect(
      (
        await request.post("/api/v1/servers/survival/members", {
          headers,
          data: { username: "viewer", role_id: role.id },
        })
      ).ok(),
    ).toBeTruthy();
    await page.goto("/#servers/survival/backups");
    const table = page.getByRole("table", {
      name: "Server backups",
      exact: true,
    });
    await expect(table.locator("tbody tr")).toHaveCount(2);
    for (const name of ["Create backup", "Upload backup"]) {
      const action = page.getByRole("button", { name, exact: true });
      if (access === "create") await expect(action).toBeEnabled();
      else await expect(action).toHaveCount(0);
    }
    const restore = table.getByRole("button", { name: "Restore", exact: true });
    if (access === "restore") {
      await expect(restore).toHaveCount(2);
      await expect(restore.first()).toBeEnabled();
    } else await expect(restore).toHaveCount(0);
    await expect(table.getByRole("button", { name: /^Delete / })).toHaveCount(
      0,
    );
    await expect(table.getByRole("link", { name: /^Download / })).toHaveCount(
      2,
    );
    if (access === "read") {
      const listing = page.locator(".table-container").filter({ has: table });
      await screenshot(page, "backup-reader.png", listing);
      await page.setViewportSize({ width: 390, height: 844 });
      await listing.scrollIntoViewIfNeeded();
      await screenshot(page, "backup-reader-mobile.png", listing);
    }
  });
}

for (const restart of [true, false]) {
  test(`running-server restore confirms data loss and restarts ${restart}`, async ({
    page,
  }) => {
    await login(page);
    await openServerTab(page, "backups", "Backups");
    const actions: string[] = [];
    page.on("request", (request) => {
      if (
        request.url().endsWith("/servers/survival/actions") &&
        request.method() === "POST"
      )
        actions.push((request.postDataJSON() as { action: string }).action);
    });
    const row = page
      .getByRole("row")
      .filter({ hasText: "mock-nightly.tar.gz" });
    await row.getByRole("button", { name: "Restore", exact: true }).click();
    const dialog = page.getByRole("dialog", {
      name: "Restore backup for Survival?",
      exact: true,
    });
    await expect(dialog).toContainText(
      "Changes made since this backup will be lost",
    );
    await expect(dialog).toContainText(
      "Connected players will be disconnected",
    );
    const checkbox = dialog.getByRole("checkbox", {
      name: "Restart server after restoring",
      exact: true,
    });
    await expect(checkbox).toBeChecked();
    if (restart) {
      await screenshot(page, "restore-running-confirm.png", dialog);
      await page.setViewportSize({ width: 320, height: 844 });
      await expect(
        dialog.getByRole("button", {
          name: "Stop, restore and restart",
          exact: true,
        }),
      ).toBeInViewport({ ratio: 1 });
    } else await checkbox.uncheck();
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    expect(actions).toEqual([]);
    await row.getByRole("button", { name: "Restore", exact: true }).click();
    if (!restart) await checkbox.uncheck();
    let releaseRestore = () => {};
    if (restart) {
      const allowed = new Promise<void>((resolve) => {
        releaseRestore = resolve;
      });
      await page.route("**/api/v1/servers/survival/actions", async (route) => {
        if (
          (route.request().postDataJSON() as { action: string }).action ===
          "restore"
        )
          await allowed;
        await route.continue();
      });
    }
    await dialog
      .getByRole("button", {
        name: restart ? "Stop, restore and restart" : "Stop and restore",
        exact: true,
      })
      .click();
    if (restart) {
      try {
        await expect(
          dialog.getByText("Restoring backup…", { exact: true }),
        ).toBeVisible();
        await expect(
          dialog.getByRole("button", { name: "Cancel", exact: true }),
        ).toBeDisabled();
        await page.keyboard.press("Escape");
        await expect(dialog).toBeVisible();
      } finally {
        releaseRestore();
      }
    }
    await expect(dialog).toHaveCount(0);
    expect(actions).toEqual(
      restart ? ["stop", "restore", "start"] : ["stop", "restore"],
    );
    await expect(
      page.getByText(
        restart
          ? "Backup restored. The server is running."
          : "Backup restored. The server is stopped.",
        { exact: true },
      ),
    ).toBeVisible();
  });
}

test("a stopped-server restore confirms the replacement and remains stopped", async ({
  page,
}) => {
  await login(page);
  await page.goto("/#servers/creative/backups");
  const row = page.getByRole("row").filter({ hasText: "mock-nightly.tar.gz" });
  await row.getByRole("button", { name: "Restore", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Restore backup for Creative?",
    exact: true,
  });
  await expect(dialog).toContainText("The server will remain stopped");
  await expect(dialog.getByRole("checkbox")).toHaveCount(0);
  await screenshot(page, "restore-stopped-confirm.png", dialog);
  await page.setViewportSize({ width: 390, height: 844 });
  await dialog.getByRole("button", { name: "Restore", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByText("Backup restored. The server is stopped.", { exact: true }),
  ).toBeVisible();
});

test("a failed restore leaves the stopped server off and reports the error", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "backups", "Backups");
  const actions: string[] = [];
  await page.route("**/api/v1/servers/survival/actions", async (route) => {
    const input = route.request().postDataJSON() as { action: string };
    actions.push(input.action);
    const response = await route.fetch();
    if (input.action !== "restore") return route.fulfill({ response });
    const status = await response.json();
    await route.fulfill({
      response,
      json: {
        ...status,
        operation_state: "failed",
        error: "The backup archive could not be restored.",
      },
    });
  });
  await page
    .getByRole("row")
    .filter({ hasText: "mock-nightly.tar.gz" })
    .getByRole("button", { name: "Restore", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Restore backup for Survival?",
    exact: true,
  });
  await dialog
    .getByRole("button", { name: "Stop, restore and restart", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toHaveText(
    "The backup archive could not be restored.",
  );
  expect(actions).toEqual(["stop", "restore"]);
  expect(
    await (await page.request.get("/api/v1/servers/survival/status")).json(),
  ).toMatchObject({ state: "stopped" });
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeEnabled();
});

for (const failed of ["stop", "start"] as const) {
  test(`a failed ${failed} reports the failed step without continuing`, async ({
    page,
  }) => {
    await login(page);
    await openServerTab(page, "backups", "Backups");
    const actions: string[] = [];
    await page.route("**/api/v1/servers/survival/actions", async (route) => {
      const action = (route.request().postDataJSON() as { action: string })
        .action;
      actions.push(action);
      if (action !== failed) return route.continue();
      const observed = await (
        await page.request.get("/api/v1/servers/survival/status")
      ).json();
      await route.fulfill({
        json: {
          ...observed,
          operation_id: `failed-${failed}`,
          operation: failed,
          operation_state: "failed",
          busy: false,
          error: `${failed} failed`,
        },
      });
    });
    await page
      .getByRole("row")
      .filter({ hasText: "mock-nightly.tar.gz" })
      .getByRole("button", { name: "Restore", exact: true })
      .click();
    const dialog = page.getByRole("dialog", {
      name: "Restore backup for Survival?",
      exact: true,
    });
    await dialog
      .getByRole("button", { name: "Stop, restore and restart", exact: true })
      .click();
    if (failed === "stop") {
      await expect(dialog.getByRole("alert")).toHaveText("stop failed");
      expect(actions).toEqual(["stop"]);
    } else {
      await expect(dialog).toHaveCount(0);
      await expect(
        page
          .getByRole("alert")
          .filter({ hasText: "Backup restored, but restarting failed" }),
      ).toBeVisible();
      expect(actions).toEqual(["stop", "restore", "start"]);
    }
    expect(
      await (await page.request.get("/api/v1/servers/survival/status")).json(),
    ).toMatchObject({ state: failed === "stop" ? "running" : "stopped" });
  });
}

test("restorers without power permission get an explanation before stopping", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/v1/servers/survival", async (route) => {
    const response = await route.fetch();
    const server = (await response.json()) as { permissions: string[] };
    await route.fulfill({
      response,
      json: {
        ...server,
        permissions: server.permissions.filter(
          (permission) => permission !== "server.operate",
        ),
      },
    });
  });
  await openServerTab(page, "backups", "Backups");
  await page
    .getByRole("row")
    .filter({ hasText: "mock-nightly.tar.gz" })
    .getByRole("button", { name: "Restore", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Restore backup for Survival?",
    exact: true,
  });
  await expect(dialog).toContainText(
    "You do not have permission to stop or restart this server",
  );
  await expect(
    dialog.getByRole("button", { name: "Restore", exact: true }),
  ).toBeDisabled();
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeEnabled();
});
