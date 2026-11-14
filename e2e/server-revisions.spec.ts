import { test, expect, screenshot } from "./fixtures";
import { login, openServerTab } from "./screens";
import type { Ember } from "../web/src/contracts/ember";

type ServerSnapshot = {
  ember_id: string;
  ember_update_available: boolean;
};

test("save-path-only Ember updates apply immediately without a restart notice", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "console", "Console");
  const { csrf_token } = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  const headers = {
    "X-CSRF-Token": csrf_token,
    Origin: "http://127.0.0.1:8099",
  };
  const server = (await (
    await page.request.get("/api/v1/servers/survival")
  ).json()) as ServerSnapshot & Record<string, unknown>;
  const ember = (await (
    await page.request.get(`/api/v1/embers/${server.ember_id}`)
  ).json()) as Ember;
  const update = await page.request.put(`/api/v1/embers/${ember.id}`, {
    headers,
    data: {
      ...ember,
      savedata_paths: [{ path: "new-save-directory", required: false }],
    },
  });
  expect(update.ok(), await update.text()).toBeTruthy();
  const current = await page.request.get("/api/v1/servers/survival");
  expect(current.ok(), await current.text()).toBeTruthy();
  expect(await current.json()).toMatchObject({
    savedata_available: true,
    ember_review_required: false,
    deployment_sync_required: false,
    ember_update_available: false,
  });
  await page.reload();
  await expect(
    page.getByRole("region", { name: "Console output", exact: true }),
  ).toBeVisible();
  const status = (await (
    await page.request.get("/api/v1/servers/survival/status")
  ).json()) as { state: string; restart_required: boolean };
  expect(status.state).toBe("running");
  expect(status.restart_required).toBe(false);
  await expect(page.getByText("Restart required", { exact: true })).toHaveCount(
    0,
  );
});

test("an older metadata poll cannot overwrite a newly saved server", async ({
  page,
}) => {
  test.setTimeout(45_000);
  await login(page);
  await openServerTab(page, "console", "Console");
  let captured!: () => void;
  const capture = new Promise<void>((resolve) => {
    captured = resolve;
  });
  let release!: () => void;
  const released = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/v1/servers/survival", async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    const response = await route.fetch();
    captured();
    await released;
    await route.fulfill({ response });
  });
  try {
    await capture;
    await page
      .getByRole("button", { name: "More server actions", exact: true })
      .click();
    await page
      .getByRole("menuitem", { name: "Edit server", exact: true })
      .click();
    const editor = page.getByRole("dialog", {
      name: "Edit server",
      exact: true,
    });
    await editor
      .getByLabel("Server name", { exact: true })
      .fill("Latest Survival");
    await editor.getByRole("button", { name: "Review", exact: true }).click();
    await editor
      .getByRole("button", { name: "Save changes", exact: true })
      .click();
    await expect(editor).toHaveCount(0);
    const stale = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/servers/survival") &&
        response.request().method() === "GET",
    );
    release();
    await (await stale).finished();
    await page.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    );
    await expect(
      page.getByRole("heading", {
        name: "Latest Survival",
        exact: true,
        level: 1,
      }),
    ).toBeVisible({ timeout: 1000 });
  } finally {
    release();
  }
});

test("pending deployment sync is distinct from Ember value review", async ({
  page,
}) => {
  await login(page);
  let pending = true;
  const actions: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST" && request.url().endsWith("/actions"))
      actions.push(request.url());
  });
  await page.route("**/api/v1/servers/survival", async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    const response = await route.fetch();
    await route.fulfill({
      response,
      json: {
        ...(await response.json()),
        ember_review_required: false,
        deployment_sync_required: pending,
        ember_update_available: pending,
      },
    });
  });
  await page.route(
    "**/api/v1/servers/survival/configuration",
    async (route) => {
      pending = false;
      await route.continue();
    },
  );
  await openServerTab(page, "console", "Console");
  const notice = page
    .locator(".ember-update-notice")
    .filter({ hasText: "Deployment sync pending" });
  await expect(notice).toBeVisible();
  await expect(notice).toContainText(
    "Saved settings are ready to sync to Spark. Container changes take effect on the next restart.",
  );
  await expect(
    notice.getByRole("button", { name: "Sync deployment", exact: true }),
  ).toBeEnabled();
  await page
    .getByRole("button", { name: "More server actions", exact: true })
    .click();
  await expect(
    page.getByRole("menuitem", { name: "Update Ember", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("menuitem", { name: "Sync deployment", exact: true })
    .click();
  await expect(notice).toHaveCount(0);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(actions).toEqual([]);
});

for (const mobile of [false, true]) {
  test(`running Ember updates and pending restarts (${mobile ? "mobile" : "desktop"})`, async ({
    page,
  }) => {
    test.setTimeout(60_000);
    await login(page);
    await openServerTab(page, "console", "Console");
    if (mobile) await page.setViewportSize({ width: 390, height: 844 });
    const session = (await (
      await page.request.get("/api/v1/auth/session")
    ).json()) as { csrf_token: string };
    const headers = {
      "X-CSRF-Token": session.csrf_token,
      Origin: "http://127.0.0.1:8099",
    };
    const server = (await (
      await page.request.get("/api/v1/servers/survival")
    ).json()) as ServerSnapshot;
    const ember = (await (
      await page.request.get(`/api/v1/embers/${server.ember_id}`)
    ).json()) as Ember;
    const update = await page.request.put(`/api/v1/embers/${ember.id}`, {
      headers,
      data: {
        ...ember,
        startup: `${ember.startup} # updated`,
        images: { "Java 25": "ghcr.io/ptero-eggs/yolks:java_25" },
        variables: [
          ...(ember.variables ?? []).map((variable) =>
            variable.name === "INSTALL_CHANNEL"
              ? { ...variable, default: "preview" }
              : variable.name === "MC_VERSION"
                ? { ...variable, default: "1.21.4" }
                : variable,
          ),
          {
            name: "ENABLE_CROSSPLAY",
            default: "1",
            required: false,
            editable: true,
            validation: { type: "boolean" },
          },
        ],
        savedata_paths: [{ path: "new-save-directory", required: false }],
      },
    });
    expect(update.ok(), await update.text()).toBeTruthy();
    // The open page discovers a manual definition edit through metadata polling.
    const notice = page
      .locator(".ember-update-notice")
      .filter({ hasText: "Ember update available" });
    await expect(notice).toBeVisible({ timeout: 15_000 });
    const suffix = mobile ? "-mobile" : "";
    await screenshot(page, `server-ember-update${suffix}.png`, notice);
    await page
      .getByRole("button", { name: "More server actions", exact: true })
      .click();
    await expect(
      page.getByRole("menuitem", { name: "Update Ember", exact: true }),
    ).toBeEnabled();
    await page.keyboard.press("Escape");
    const actions: string[] = [];
    page.on("request", (request) => {
      if (
        request.method() === "POST" &&
        request.url().endsWith("/servers/survival/actions")
      )
        actions.push(request.postData() ?? "");
    });
    await notice
      .getByRole("button", { name: "Review Ember update", exact: true })
      .click();
    const editor = page.getByRole("dialog", {
      name: "Update Ember",
      exact: true,
    });
    await expect(
      editor.getByText("Changes are saved without stopping the server.", {
        exact: false,
      }),
    ).toBeVisible();
    await expect(
      editor.getByRole("combobox", { name: "Image", exact: true }),
    ).toHaveAccessibleDescription("Changed");
    // The inherited startup changed in the Ember, but there is no saved override change.
    await expect(
      editor.getByLabel("Startup command text", { exact: true }),
    ).not.toHaveAccessibleDescription(/Changed/);
    await expect(
      editor.getByRole("button", { name: "General", exact: true }),
    ).toHaveAccessibleDescription("Contains changes");
    await expect(
      editor.getByRole("button", { name: "Variables", exact: true }),
    ).toHaveAccessibleDescription("Contains changes");
    await expect(
      editor.getByRole("button", { name: "Resources & ports", exact: true }),
    ).not.toHaveAccessibleDescription("Contains changes");
    await screenshot(page, `server-ember-revision${suffix}.png`, editor);
    await editor
      .getByRole("button", { name: "Variables", exact: true })
      .click();
    await expect(
      editor.getByLabel("INSTALL_CHANNEL", { exact: true }),
    ).toHaveAccessibleDescription(/Changed/);
    await expect(
      editor.getByLabel("INSTALL_CHANNEL", { exact: true }),
    ).toHaveValue("preview");
    await expect(
      editor.getByRole("switch", { name: "ENABLE_CROSSPLAY", exact: true }),
    ).toHaveAccessibleDescription("Added");
    await expect(
      editor.getByLabel("MC_VERSION", { exact: true }),
    ).not.toHaveAccessibleDescription(/Changed/);
    await expect(editor.getByLabel("MC_VERSION", { exact: true })).toHaveValue(
      "latest",
    );
    await editor.getByRole("button", { name: "General", exact: true }).click();
    await editor
      .getByLabel("Startup command text", { exact: true })
      .fill("java -jar custom.jar");
    await expect(
      editor.getByLabel("Startup command text", { exact: true }),
    ).toHaveAccessibleDescription(/Changed/);
    await editor
      .getByRole("button", { name: "Use Ember default", exact: true })
      .click();
    await expect(
      editor.getByLabel("Startup command text", { exact: true }),
    ).not.toHaveAccessibleDescription(/Changed/);
    await editor
      .getByLabel("Server name", { exact: true })
      .fill("Temporary name");
    await expect(
      editor.getByLabel("Server name", { exact: true }),
    ).toHaveAccessibleDescription("Changed");
    await editor.getByLabel("Server name", { exact: true }).fill("Survival");
    await expect(
      editor.getByLabel("Server name", { exact: true }),
    ).not.toHaveAccessibleDescription("Changed");
    await editor
      .getByRole("button", { name: "Resources & ports", exact: true })
      .click();
    const cpu = editor.getByRole("spinbutton", {
      name: "CPU cores",
      exact: true,
    });
    await cpu.fill("3");
    await expect(cpu).toHaveAccessibleDescription("Changed");
    await editor.getByRole("button", { name: "Review", exact: true }).click();
    await expect(editor.getByText("Changed", { exact: true })).toHaveCount(3);
    await expect(editor.getByText("Added", { exact: true })).toHaveCount(1);
    if (!mobile)
      await screenshot(page, "server-ember-revision-review.png", editor);
    await editor
      .getByRole("button", { name: "Update Ember", exact: true })
      .click();
    await expect(editor).toHaveCount(0);
    await expect(notice).toHaveCount(0);
    const pending = page
      .locator(".ember-update-notice")
      .filter({ hasText: "Restart required" });
    await expect(pending).toBeVisible();
    await screenshot(page, `server-restart-required${suffix}.png`, pending);
    expect(actions).toEqual([]);
    const running = (await (
      await page.request.get("/api/v1/servers/survival/status")
    ).json()) as { state: string };
    expect(running.state).toBe("running");
    await page.reload();
    await expect(pending).toBeVisible();
    await page.getByRole("button", { name: "Restart", exact: true }).click();
    await expect(pending).toHaveCount(0, { timeout: 20_000 });
    const saved = (await (
      await page.request.get("/api/v1/servers/survival")
    ).json()) as ServerSnapshot;
    const applied = (await (
      await page.request.get("/api/v1/servers/survival/status")
    ).json()) as { restart_required: boolean };
    expect(applied.restart_required).toBe(false);
    expect(saved.ember_update_available).toBe(false);
  });
}

test("ordinary runtime edits request a restart and metadata edits do not", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "console", "Console");
  async function openEditor() {
    await page
      .getByRole("button", { name: "More server actions", exact: true })
      .click();
    await page
      .getByRole("menuitem", { name: "Edit server", exact: true })
      .click();
    return page.getByRole("dialog", { name: "Edit server", exact: true });
  }
  let editor = await openEditor();
  await editor
    .getByLabel("Server name", { exact: true })
    .fill("Renamed Survival");
  await editor.getByRole("button", { name: "Review", exact: true }).click();
  await editor
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(editor).toHaveCount(0);
  const pending = page.getByText("Restart required", { exact: true });
  await expect(pending).toHaveCount(0);
  editor = await openEditor();
  const command = editor.getByLabel("Startup command text", { exact: true });
  const originalCommand = await command.inputValue();
  await command.fill("sleep 4000");
  await editor
    .getByRole("button", { name: "Use Ember default", exact: true })
    .click();
  await expect(command).toHaveValue(originalCommand);
  await command.fill("sleep 4000");
  await editor.getByRole("button", { name: "Review", exact: true }).click();
  await editor
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(editor).toHaveCount(0);
  await expect(pending).toBeVisible();
});
