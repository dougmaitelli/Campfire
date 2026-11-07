import { test, expect, screenshot } from "./fixtures";
import { login, openServerTab, openScreen, inspectServer } from "./screens";

test("overview and server lists poll one aggregate runtime endpoint", async ({
  page,
}) => {
  const individual: string[] = [];
  page.on("request", (request) => {
    if (
      /\/api\/v1\/servers\/[^/]+\/status$/.test(new URL(request.url()).pathname)
    )
      individual.push(request.url());
  });
  await login(page);
  const snapshot = page.waitForResponse((response) =>
    response.url().endsWith("/api/v1/servers/status"),
  );
  await openScreen(page, "servers", "Servers");
  expect((await snapshot).ok()).toBeTruthy();
  expect(individual).toEqual([]);
});

for (const mobile of [false, true]) {
  test(`returning to Servers after opening a console from Overview shows the list (${mobile ? "mobile" : "desktop"})`, async ({
    page,
  }) => {
    await login(page);
    if (mobile) await page.setViewportSize({ width: 390, height: 844 });
    async function navigate(label: string) {
      if (mobile)
        await page
          .getByRole("button", { name: "Open navigation", exact: true })
          .click();
      await page
        .getByRole("navigation", { name: "Main navigation", exact: true })
        .getByRole("link", { name: label, exact: true })
        .click();
      await expect(
        page.getByRole("heading", { name: label, exact: true, level: 1 }),
      ).toBeVisible();
    }

    for (const [destination, heading] of [
      ["schedules", "Schedules"],
      ["sparks", "Sparks"],
    ] as const) {
      await navigate("Overview");
      await page
        .getByRole("button", { name: "Open Survival", exact: true })
        .click();
      await expect(page).toHaveURL(/#servers\/survival\/console$/);
      await expect(
        page.getByRole("region", { name: "Console output", exact: true }),
      ).toBeVisible();

      await navigate(heading);
      await navigate("Servers");
      await expect(page).toHaveURL(/#servers$/);
      await expect(
        page.getByRole("table", { name: "Game servers", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("region", { name: "Console output", exact: true }),
      ).toHaveCount(0);

      await page.goBack();
      await expect(page).toHaveURL(new RegExp(`#${destination}$`));
      await expect(
        page.getByRole("heading", { name: heading, exact: true, level: 1 }),
      ).toBeVisible();
      await page.goForward();
      await expect(page).toHaveURL(/#servers$/);
      await expect(
        page.getByRole("table", { name: "Game servers", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("region", { name: "Console output", exact: true }),
      ).toHaveCount(0);
    }
  });
}

test("unsaved file edits can block or allow navigation", async ({ page }) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  const editor = page.getByRole("textbox", {
    name: "/server.properties",
    exact: true,
  });
  const original = await editor.inputValue();
  const changed = original + "\n# unsaved changes";
  await editor.fill(changed);
  await expect(
    page.getByRole("status").filter({ hasText: "Unsaved changes" }),
  ).toBeVisible();
  const schedules = page
    .getByRole("navigation", { name: "Main navigation", exact: true })
    .getByRole("link", { name: "Schedules", exact: true });
  page.once("dialog", (dialog) => void dialog.dismiss());
  await schedules.click();
  await expect(page).toHaveURL(/#servers\/survival\/files$/);
  await expect(editor).toHaveValue(changed);

  page.once("dialog", (dialog) => void dialog.accept());
  await schedules.click();
  await expect(page).toHaveURL(/#schedules$/);
  await expect(
    page.getByRole("heading", { name: "Schedules", exact: true, level: 1 }),
  ).toBeVisible();
  await page.goto("/#servers/survival/files");
  await page
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  await expect(editor).toHaveValue(original);
});

for (const mobile of [false, true]) {
  test(`server rows open the console with three power controls (${mobile ? "mobile" : "desktop"})`, async ({
    page,
  }) => {
    await login(page);
    await openScreen(page, "servers", "Servers");
    if (mobile) await page.setViewportSize({ width: 390, height: 844 });
    const row = page.getByRole("row").filter({
      has: page.getByRole("button", { name: "Open Survival", exact: true }),
    });
    for (const action of ["Start", "Stop", "Restart"])
      await expect(
        row.getByRole("button", { name: `${action} Survival`, exact: true }),
      ).toBeVisible();
    await expect(
      row.getByRole("link", { name: "Open console for Survival", exact: true }),
    ).toHaveCount(0);
    await row
      .getByRole("button", { name: "Open Survival", exact: true })
      .press("Enter");
    await expect(page).toHaveURL(/#servers\/survival\/console$/);
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(
      page.getByRole("region", { name: "Console output", exact: true }),
    ).toBeVisible();
  });
}

test("server page actions edit configuration directly and keep the console open", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "console", "Console");
  await page
    .getByRole("button", { name: "More server actions", exact: true })
    .click();
  await expect(
    page
      .getByRole("menu", { name: "More server actions", exact: true })
      .getByRole("menuitem")
      .last(),
  ).toHaveText("Delete");
  await page
    .getByRole("menuitem", { name: "Edit server", exact: true })
    .click();
  const editor = page.getByRole("dialog", { name: "Edit server", exact: true });
  await expect(page.getByRole("dialog")).toHaveCount(1);
  await editor
    .getByLabel("Server name", { exact: true })
    .fill("Updated Survival");
  await editor.getByRole("button", { name: "Review", exact: true }).click();
  await editor
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(editor).toHaveCount(0);
  await expect(page).toHaveURL(/#servers\/survival\/console$/);
  await expect(
    page.getByRole("heading", {
      level: 1,
      name: "Updated Survival",
      exact: true,
    }),
  ).toBeVisible();
});

for (const allowed of [false, true]) {
  test(`server drawer console shortcut respects console-read access ${allowed}`, async ({
    page,
  }) => {
    await login(page);
    await page.route("**/api/v1/servers/survival", async (route) => {
      const response = await route.fetch();
      const server = (await response.json()) as Record<string, unknown>;
      await route.fulfill({
        response,
        json: {
          ...server,
          permissions: [
            "server.view",
            ...(allowed ? ["server.console.read"] : []),
          ],
          can_manage_access: false,
        },
      });
    });
    await inspectServer(page);
    const drawer = page.getByRole("dialog", { name: "Server", exact: true });
    const consoleLink = drawer.getByRole("link", {
      name: "Open console for Survival",
      exact: true,
    });
    if (allowed) {
      await expect(consoleLink).toBeVisible();
      await consoleLink.click();
      await expect(drawer).toHaveCount(0);
      await expect(
        page.getByRole("region", { name: "Console output", exact: true }),
      ).toBeVisible();
    } else {
      await expect(consoleLink).toHaveCount(0);
      await drawer
        .getByRole("button", { name: "Close", exact: true })
        .first()
        .click();
      await expect(page).toHaveURL(/#schedules$/);
    }
  });
}

test("view-only server rows hide power actions and fall back to an allowed tab", async ({
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
      name: "Overview only",
      scope: "server",
      permissions: ["server.view"],
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
  await page.reload();
  for (const [screen, heading] of [
    ["dashboard", "Overview"],
    ["servers", "Servers"],
  ] as const) {
    await openScreen(page, screen, heading);
    const row = page.getByRole("row").filter({
      has: page.getByRole("button", { name: "Open Survival", exact: true }),
    });
    await expect(row).toBeVisible();
    for (const action of ["Start", "Stop", "Restart"])
      await expect(
        row.getByRole("button", { name: `${action} Survival`, exact: true }),
      ).toHaveCount(0);
    if (screen === "servers") {
      const listing = page.locator(".table-container").filter({ has: row });
      await screenshot(page, "servers-view-only.png", listing);
      await page.setViewportSize({ width: 390, height: 844 });
      await listing.scrollIntoViewIfNeeded();
      await screenshot(page, "servers-view-only-mobile.png", listing);
    }
  }
  await page
    .getByRole("button", { name: "Open Survival", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Game activity", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Console", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "More server actions", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("menuitem", { name: "Edit server", exact: true }),
  ).toHaveCount(0);
});
