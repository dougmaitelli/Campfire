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
  const configuration = Object.fromEntries(
    [
      "name",
      "description",
      "spark_id",
      "ember_id",
      "image",
      "startup_command",
      "startup_override",
      "game_integration",
      "variables",
      "cpu_limit",
      "memory_mb",
      "disk_mb",
      "ports",
    ].map((key) => [key, server[key]]),
  );
  const applied = await page.request.post("/api/v1/servers/survival/ember", {
    headers,
    data: { ...configuration, expected_ember_id: server.ember_id },
  });
  expect(applied.ok(), await applied.text()).toBeTruthy();
  expect(
    ((await applied.json()) as { savedata_available: boolean })
      .savedata_available,
  ).toBe(true);
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
    await screenshot(page, `server-ember-revision${suffix}.png`, editor);
    await editor.getByRole("button", { name: "Review", exact: true }).click();
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
  await editor
    .getByLabel("Startup command text", { exact: true })
    .fill("sleep 4000");
  await editor.getByRole("button", { name: "Review", exact: true }).click();
  await editor
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(editor).toHaveCount(0);
  await expect(pending).toBeVisible();
});
