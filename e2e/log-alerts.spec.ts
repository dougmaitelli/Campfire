import { test, expect, prepareScreenshot } from "./fixtures";
import { login, openScreen } from "./screens";
import type { Ember } from "../web/src/contracts/ember";
import type { DetectedLogAlert } from "../web/src/contracts/logAlerts";

test("actionable errors can be authored, detected without an open console, acted on and dismissed", async ({
  page,
  baseURL,
}) => {
  test.setTimeout(60_000);
  await login(page);
  const auth = await page.request.post("/api/v1/auth/login", {
    data: { username: "admin", password: "campfire-e2e-password" },
  });
  const { csrf_token } = (await auth.json()) as { csrf_token: string };
  const headers = { "X-CSRF-Token": csrf_token, Origin: baseURL! };
  const imported = await page.request.post("/api/v1/embers", {
    headers,
    data: {
      definition: JSON.stringify({
        schema: 1,
        name: "Actionable errors fixture",
        description: "",
        images: { Alpine: "alpine:3.22" },
        startup: "sleep 3600",
        work_dir: "/home/container",
        user: "1000:1000",
        variables: [],
      }),
    },
  });
  expect(imported.ok()).toBeTruthy();
  const ember = (await imported.json()) as Ember;
  await page.reload();
  await openScreen(page, "embers", "Embers");
  await page
    .getByRole("button", {
      name: "Edit Ember Actionable errors fixture",
      exact: true,
    })
    .click();
  const drawer = page.getByRole("dialog", { name: "Edit Ember", exact: true });
  await drawer.getByRole("button", { name: "Runtime", exact: true }).click();
  await drawer
    .getByRole("button", { name: "Add error rule", exact: true })
    .click();
  const first = drawer.getByRole("group", {
    name: "Error rule 1",
    exact: true,
  });
  await first
    .getByLabel("Issue title", { exact: true })
    .fill("EULA requires acceptance");
  await first
    .getByLabel("What the user should do", { exact: true })
    .fill("Read the EULA, then update eula.txt if you agree.");
  await first
    .getByLabel("Message or pattern", { exact: true })
    .fill("agree to the EULA");
  const condition = first.getByRole("group", {
    name: "Error condition 1",
    exact: true,
  });
  await condition.getByLabel("Ignore letter case", { exact: true }).check();
  await expect(
    drawer.getByLabel("Ignore console color codes", { exact: true }),
  ).toHaveCount(0);
  await first
    .getByRole("combobox", { name: "Monitor output from", exact: true })
    .click();
  await page.getByRole("option", { name: "Runtime", exact: true }).click();
  await first
    .getByRole("combobox", { name: "Action button", exact: true })
    .click();
  await page.getByRole("option", { name: "Open a file", exact: true }).click();
  await first
    .getByLabel("Configuration file path", { exact: true })
    .fill("eula.txt");
  await page.setViewportSize({ width: 390, height: 844 });
  await condition
    .getByLabel("Use a regular expression", { exact: true })
    .scrollIntoViewIfNeeded();
  const inputBottom = (await condition
    .getByRole("textbox", { name: "Message or pattern", exact: true })
    .boundingBox())!;
  const checkboxTop = (await condition
    .getByText("Use a regular expression", { exact: true })
    .boundingBox())!;
  expect(
    checkboxTop.y - inputBottom.y - inputBottom.height,
  ).toBeLessThanOrEqual(14);
  await page.setViewportSize({ width: 1400, height: 900 });

  await drawer
    .getByRole("button", { name: "Add error rule", exact: true })
    .click();
  const second = drawer.getByRole("group", {
    name: "Error rule 2",
    exact: true,
  });
  await second
    .getByLabel("Issue title", { exact: true })
    .fill("Java version needs changing");
  await second
    .getByLabel("What the user should do", { exact: true })
    .fill("Choose a compatible Java image in server settings.");
  await second
    .getByLabel("Message or pattern", { exact: true })
    .fill("UnsupportedClassVersionError");
  await second
    .getByRole("combobox", { name: "Action button", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Server settings", exact: true })
    .click();
  await drawer.getByRole("tab", { name: "JSON / YAML", exact: true }).click();
  const edited = JSON.parse(
    await drawer
      .getByRole("textbox", { name: "Definition", exact: true })
      .inputValue(),
  ) as Ember;
  expect(edited.log_alerts).toHaveLength(2);
  expect(edited.log_alerts![0]).not.toHaveProperty("id");
  expect(edited.log_alerts![0]).not.toHaveProperty("ignore_case");
  expect(edited.log_alerts![0]).not.toHaveProperty("strip_ansi");
  expect(edited.log_alerts![0]!.any).toEqual([
    { value: "agree to the EULA", ignore_case: true },
  ]);
  expect(edited.log_alerts![0]!.action).toEqual({
    type: "open_file",
    path: "eula.txt",
  });
  await drawer.getByRole("tab", { name: "Visual editor", exact: true }).click();
  await expect(
    drawer.getByRole("heading", { name: "Runtime", exact: true }),
  ).toBeVisible();
  await drawer.getByRole("button", { name: "Review", exact: true }).click();
  await drawer.getByRole("button", { name: "Save Ember", exact: true }).click();
  await expect(drawer).toHaveCount(0);

  const created = await page.request.post("/api/v1/servers", {
    headers,
    data: {
      name: "Actionable-error server",
      description: "",
      spark_id: "mock-europe",
      ember_id: ember.id,
      image: "alpine:3.22",
      variables: {},
      cpu_limit: 0.5,
      memory_mb: 512,
      disk_mb: 0,
      ports: [],
    },
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  const server = (await created.json()) as { id: string };
  const endpoint = `/api/v1/servers/${server.id}`;
  expect(
    (
      await page.request.post(`${endpoint}/actions`, {
        headers,
        data: { action: "install" },
      })
    ).ok(),
  ).toBeTruthy();
  expect(
    (
      await page.request.post(`${endpoint}/actions`, {
        headers,
        data: { action: "start" },
      })
    ).ok(),
  ).toBeTruthy();
  await expect
    .poll(
      async () =>
        (await (await page.request.get(`${endpoint}/status`)).json()).state,
    )
    .toBe("running");
  for (let i = 0; i < 2; i++)
    expect(
      (
        await page.request.post(`${endpoint}/console`, {
          headers,
          data: { command: "You need to AGREE TO THE EULA" },
        })
      ).ok(),
    ).toBeTruthy();
  let issues = (await (
    await page.request.get(`${endpoint}/log-alerts`)
  ).json()) as DetectedLogAlert[];
  expect(issues).toHaveLength(1);
  const firstID = issues[0]!.id;
  const inbox = (await (
    await page.request.get("/api/v1/notifications")
  ).json()) as { title: string; body: string }[];
  expect(
    inbox.filter(
      (item) =>
        item.title === "Actionable-error server: EULA requires acceptance",
    ),
  ).toHaveLength(1);

  // Freeze only the displayed observation time; detection and persistence above use the real API.
  await page.route(`**${endpoint}/log-alerts`, async (route) => {
    const response = await route.fetch();
    const value = (await response.json()) as DetectedLogAlert[];
    await route.fulfill({
      response,
      json: value.map((item) => ({ ...item, detected_at: 1792670400 })),
    });
  });
  await page.goto(`/#servers/${server.id}/console`);
  const attention = page.getByRole("region", {
    name: "Needs attention",
    exact: true,
  });
  await expect(
    attention.getByText("EULA requires acceptance", { exact: true }),
  ).toBeVisible();
  await attention.getByText("Matched console output", { exact: true }).click();
  await prepareScreenshot(page);
  await expect(attention).toHaveScreenshot(["crops", "server-log-alert.png"]);
  await page.setViewportSize({ width: 390, height: 844 });
  await prepareScreenshot(page);
  await expect(attention).toHaveScreenshot([
    "crops",
    "server-log-alert-mobile.png",
  ]);
  await attention
    .getByRole("link", { name: "Open eula.txt", exact: true })
    .click();
  await expect(
    page.getByRole("textbox", { name: "/eula.txt", exact: true }),
  ).toHaveValue(/eula=false/);
  await attention.getByRole("button", { name: "Dismiss", exact: true }).click();
  await expect(attention).toHaveCount(0);
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Actionable-error server", exact: true }),
  ).toBeVisible();
  expect(
    await (await page.request.get(`${endpoint}/log-alerts`)).json(),
  ).toEqual([]);

  expect(
    (
      await page.request.post(`${endpoint}/console`, {
        headers,
        data: { command: "UnsupportedClassVersionError" },
      })
    ).ok(),
  ).toBeTruthy();
  await expect(
    attention.getByText("Java version needs changing", { exact: true }),
  ).toBeVisible();
  await attention
    .getByRole("button", { name: "Server settings", exact: true })
    .click();
  const settings = page.getByRole("dialog", {
    name: "Edit server",
    exact: true,
  });
  await expect(settings).toBeVisible();
  await settings
    .getByRole("textbox", { name: "Server name", exact: true })
    .fill("Unsaved name");
  const current = (await (
    await page.request.get(`${endpoint}/log-alerts`)
  ).json()) as DetectedLogAlert[];
  expect(
    (
      await page.request.post(
        `${endpoint}/log-alerts/${current[0]!.id}/dismiss`,
        { headers, data: {} },
      )
    ).ok(),
  ).toBeTruthy();
  await expect(attention).toHaveCount(0);
  await expect(
    settings.getByRole("textbox", { name: "Server name", exact: true }),
  ).toHaveValue("Unsaved name");
  await settings.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(settings).toHaveCount(0);

  expect(
    (
      await page.request.post(`${endpoint}/actions`, {
        headers,
        data: { action: "restart" },
      })
    ).ok(),
  ).toBeTruthy();
  await expect
    .poll(
      async () =>
        (await (await page.request.get(`${endpoint}/status`)).json()).state,
    )
    .toBe("running");
  expect(
    (
      await page.request.post(`${endpoint}/console`, {
        headers,
        data: { command: "You need to agree to the EULA" },
      })
    ).ok(),
  ).toBeTruthy();
  issues = (await (
    await page.request.get(`${endpoint}/log-alerts`)
  ).json()) as DetectedLogAlert[];
  expect(issues).toHaveLength(1);
  expect(issues[0]!.id).not.toBe(firstID);
});
