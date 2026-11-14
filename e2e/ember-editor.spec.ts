import { readFile } from "node:fs/promises";
import { test, expect, prepareScreenshot } from "./fixtures";
import { login, openScreen } from "./screens";
import { completeEmber } from "../web/tests/fixtures/ember";
import {
  LogAction,
  LogPhase,
  LogSeverity,
} from "../web/src/contracts/logAlerts";

test("Ember wizard screenshots cover whole steps and representative mobile layouts", async ({
  page,
}) => {
  test.setTimeout(60_000);
  await login(page);
  await openScreen(page, "embers", "Embers");
  await page
    .getByRole("button", { name: "+ Create Ember", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Create Ember",
    exact: true,
  });
  await drawer.getByRole("tab", { name: "JSON / YAML", exact: true }).click();
  await drawer.getByLabel("Definition", { exact: true }).fill(
    JSON.stringify({
      ...completeEmber(),
      artUrl: "",
      origin: undefined,
      log_alerts: [
        {
          title: "EULA requires acceptance",
          message: "Read the EULA, then update eula.txt if you agree.",
          severity: LogSeverity.Error,
          phase: LogPhase.Runtime,
          any: [{ value: "agree to the EULA", ignore_case: true }],
          action: { type: LogAction.OpenFile, path: "eula.txt" },
        },
      ],
    }),
  );
  await drawer.getByRole("tab", { name: "Visual editor", exact: true }).click();
  await expect(drawer.getByLabel("Name", { exact: true })).toHaveValue(
    "Authoring fixture",
  );

  const steps = [
    ["basics", "Basics", false],
    ["installation", "Installation", false],
    ["runtime", "Runtime", true],
    ["variables", "Variables", true],
    ["ports", "Ports", false],
    ["files", "Files", false],
    ["storage", "Storage", false],
    ["review", "Review", false],
  ] as const;
  for (const [index, [id, title, mobile]] of steps.entries()) {
    await page.setViewportSize({ width: 1400, height: 900 });
    if (index > 0)
      await drawer
        .getByRole("button", { name: `Next: ${title}`, exact: true })
        .click();
    await expect(
      drawer.getByRole("heading", { level: 3, name: title, exact: true }),
    ).toBeVisible();
    const layouts = [
      { width: 1400, height: 900, suffix: "" },
      { width: 390, height: 844, suffix: "-mobile" },
    ];
    for (const layout of layouts) {
      await page.setViewportSize({
        width: layout.width,
        height: layout.height,
      });
      if (layout.suffix && !mobile) continue;
      await prepareScreenshot(page);
      // Expand vertically so one image includes the header, complete step, and
      // footer. Interaction tests retain the normal phone viewport and scrolling.
      const height = await drawer.evaluate((element) => element.scrollHeight);
      await page.setViewportSize({
        width: layout.width,
        height: Math.max(layout.height, height),
      });
      await drawer.evaluate((element) => element.scrollTo(0, 0));
      await expect(drawer).toHaveScreenshot([
        "crops",
        `ember-editor-${id}${layout.suffix}.png`,
      ]);
    }
  }
});

test("a new Ember can be built and saved through visual fields on desktop and mobile", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "embers", "Embers");
  await page
    .getByRole("button", { name: "+ Create Ember", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Create Ember",
    exact: true,
  });
  await expect(
    drawer.getByRole("button", { name: "Next: Installation", exact: true }),
  ).toBeDisabled();
  await drawer.getByLabel("Name", { exact: true }).fill("Visual game");
  await drawer
    .getByLabel("Description", { exact: true })
    .fill("Built with visual fields.");
  await drawer
    .getByRole("button", { name: "Next: Installation", exact: true })
    .click();
  await expect(
    drawer.getByRole("heading", { name: "Installation", exact: true }),
  ).toHaveCount(1);
  await drawer
    .getByRole("checkbox", { name: "Run an installation script", exact: true })
    .check();
  await drawer
    .getByLabel("Installer image", { exact: true })
    .fill("alpine:3.22");
  await drawer
    .getByLabel("Installation script", { exact: true })
    .fill("echo install");
  await drawer
    .getByRole("button", { name: "Next: Runtime", exact: true })
    .click();
  await drawer.getByRole("button", { name: "Add image", exact: true }).click();
  await drawer.getByLabel("Image label", { exact: true }).fill("Alpine");
  await drawer.getByLabel("Image address", { exact: true }).fill("alpine:3.22");
  await drawer.getByRole("tab", { name: "Visual editor", exact: true }).focus();
  await page.keyboard.press("ArrowRight");
  await expect(
    drawer.getByRole("tab", { name: "JSON / YAML", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await page.keyboard.press("ArrowLeft");
  await expect(drawer.getByRole("heading", { level: 3 })).toHaveText("Runtime");
  await drawer
    .getByLabel("Startup command", { exact: true })
    .fill("echo Ready; sleep {{SECONDS}}");
  await drawer
    .getByLabel("Stop command or signal", { exact: true })
    .fill("^SIGTERM");
  await drawer
    .getByRole("combobox", { name: "Mark the server ready", exact: true })
    .click();
  await page
    .getByRole("option", {
      name: "When a message or pattern matches",
      exact: true,
    })
    .click();
  await drawer.getByLabel("Message or pattern", { exact: true }).fill("Ready");
  await drawer
    .getByRole("button", { name: "Next: Variables", exact: true })
    .click();
  await drawer
    .getByRole("button", { name: "Add variable", exact: true })
    .click();
  await drawer.getByLabel("Variable name", { exact: true }).fill("SECONDS");
  await drawer.getByLabel("Default value", { exact: true }).fill("30");
  await drawer.getByRole("checkbox", { name: "Required", exact: true }).check();
  await drawer
    .getByRole("checkbox", { name: "Add validation rules", exact: true })
    .check();
  await drawer
    .getByRole("combobox", { name: "Value type", exact: true })
    .click();
  await page.getByRole("option", { name: "Integer", exact: true }).click();
  await drawer.getByLabel("Minimum value", { exact: true }).fill("1");
  await drawer.getByLabel("Maximum value", { exact: true }).fill("120");
  await drawer
    .getByRole("button", { name: "Next: Ports", exact: true })
    .click();
  await drawer.getByRole("button", { name: "Add port", exact: true }).click();
  await drawer.getByLabel("Container port", { exact: true }).fill("25565");
  await drawer.getByLabel("Description", { exact: true }).fill("Game");
  const port = drawer.getByRole("group", { name: "port 1", exact: true });
  await drawer.getByRole("button", { name: "Add port", exact: true }).click();
  await drawer
    .getByRole("button", { name: "Remove port 2", exact: true })
    .click();
  await expect(port.getByLabel("Container port", { exact: true })).toHaveValue(
    "25565",
  );
  await drawer
    .getByRole("button", { name: "Next: Files", exact: true })
    .click();
  await drawer
    .getByRole("button", { name: "Add configuration file", exact: true })
    .click();
  await drawer
    .getByLabel("Configuration file path", { exact: true })
    .fill("server.properties");
  await drawer
    .getByRole("button", { name: "Add replacement", exact: true })
    .click();
  await drawer
    .getByLabel("Key or search text", { exact: true })
    .fill("server-port");
  await drawer
    .getByLabel("Replacement value", { exact: true })
    .fill("{{SERVER_PORT}}");
  await drawer
    .getByRole("button", { name: "Next: Storage", exact: true })
    .click();
  await drawer
    .getByRole("button", { name: "Add save path", exact: true })
    .click();
  await drawer.getByLabel("Relative path", { exact: true }).fill("world");
  await drawer
    .getByRole("checkbox", {
      name: "Fail a backup if this path is missing",
      exact: true,
    })
    .check();
  const savePath = drawer.getByRole("group", {
    name: "save path 1",
    exact: true,
  });
  await drawer
    .getByRole("button", { name: "Add save path", exact: true })
    .click();
  await drawer
    .getByRole("button", { name: "Remove save path 2", exact: true })
    .click();
  await expect(
    savePath.getByLabel("Relative path", { exact: true }),
  ).toHaveValue("world");
  await drawer
    .getByRole("button", { name: "Next: Review", exact: true })
    .click();
  const save = drawer.getByRole("button", {
    name: "Create Ember",
    exact: true,
  });
  await expect(save).toBeEnabled();
  await expect(
    drawer.getByText("Converted Ember", { exact: true }),
  ).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 });
  const response = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/embers") &&
      response.request().method() === "POST",
  );
  await save.click();
  const saved = await response;
  expect(saved.ok()).toBeTruthy();
  expect(await saved.json()).toMatchObject({
    name: "Visual game",
    startup: "echo Ready; sleep {{SECONDS}}",
    ready: "Ready",
    stop: "^SIGTERM",
    images: { Alpine: "alpine:3.22" },
    installer: {
      image: "alpine:3.22",
      entrypoint: "sh",
      script: "echo install",
    },
    variables: [
      {
        name: "SECONDS",
        default: "30",
        validation: { type: "integer", min: 1, max: 120 },
      },
    ],
    configs: [
      {
        path: "server.properties",
        parser: "properties",
        values: { "server-port": "{{SERVER_PORT}}" },
        create: true,
      },
    ],
    required_ports: [
      { container: 25565, protocol: "tcp", description: "Game" },
    ],
    savedata_paths: [{ path: "world", required: true }],
  });
  await page.reload();
  await page
    .getByRole("button", { name: "Edit Ember Visual game", exact: true })
    .click();
  await expect(
    page.getByRole("dialog").getByLabel("Name", { exact: true }),
  ).toHaveValue("Visual game");
});

test("editing advanced fields and pasting JSON preserves the entire definition", async ({
  page,
  baseURL,
}) => {
  await login(page);
  const auth = await page.request.post("/api/v1/auth/login", {
    data: { username: "admin", password: "campfire-e2e-password" },
  });
  const { csrf_token } = (await auth.json()) as { csrf_token: string };
  const original = { ...completeEmber(), artUrl: "" };
  const created = await page.request.post("/api/v1/embers", {
    headers: { "X-CSRF-Token": csrf_token, Origin: baseURL! },
    data: { definition: JSON.stringify(original), acknowledge_warnings: true },
  });
  expect(created.ok()).toBeTruthy();
  const stored = (await created.json()) as { id: string };
  await page.reload();
  await openScreen(page, "embers", "Embers");
  await page
    .getByRole("button", { name: "Edit Ember Authoring fixture", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Edit Ember", exact: true });
  await drawer.getByRole("button", { name: "Runtime", exact: true }).click();
  const debug = drawer
    .getByRole("group", { name: /^startup command \d+$/ })
    .filter({ has: page.locator('input[value="Debug"]') });
  await expect(debug.getByLabel("Command name", { exact: true })).toHaveValue(
    "Debug",
  );
  await debug
    .getByLabel("Startup command", { exact: true })
    .fill("echo Updated; sleep {{SECONDS}}");
  const rule = drawer.getByRole("group", {
    name: "readiness condition 2",
    exact: true,
  });
  await rule
    .getByLabel("Message or pattern", { exact: true })
    .fill("Now listening");
  const regex = rule.getByRole("checkbox", {
    name: "Use a regular expression",
    exact: true,
  });
  await expect(regex).not.toBeChecked();
  await regex.check();
  await regex.uncheck();
  await drawer
    .getByRole("combobox", { name: "Mark the server ready", exact: true })
    .click();
  await page
    .getByRole("option", { name: "When the container is running", exact: true })
    .click();
  await expect(rule).toHaveCount(0);
  await drawer.getByRole("tab", { name: "JSON / YAML", exact: true }).click();
  const runningDefinition = JSON.parse(
    await drawer.getByLabel("Definition", { exact: true }).inputValue(),
  );
  expect(runningDefinition.ready).toBeUndefined();
  expect(runningDefinition.readiness).toBeUndefined();
  await drawer.getByRole("tab", { name: "Visual editor", exact: true }).click();
  await expect(drawer.getByRole("heading", { level: 3 })).toHaveText("Runtime");
  await expect(
    drawer.getByRole("combobox", {
      name: "Mark the server ready",
      exact: true,
    }),
  ).toHaveText("When the container is running");
  await drawer
    .getByRole("combobox", { name: "Mark the server ready", exact: true })
    .click();
  await page
    .getByRole("option", {
      name: "When a message or pattern matches",
      exact: true,
    })
    .click();
  await expect(
    rule.getByLabel("Message or pattern", { exact: true }),
  ).toHaveValue("Now listening");
  await drawer.getByRole("button", { name: "Variables", exact: true }).click();
  const seconds = drawer.getByRole("group", { name: "SECONDS", exact: true });
  await seconds.getByLabel("Maximum value", { exact: true }).fill("180");
  await seconds
    .getByRole("button", { name: "Add allowed value", exact: true })
    .click();
  await seconds
    .getByRole("button", { name: "Remove allowed value 3", exact: true })
    .click();
  const urlVariable = drawer.getByRole("group", { name: "URL", exact: true });
  await expect(
    urlVariable.getByRole("combobox", { name: "Value type", exact: true }),
  ).toHaveText("URL");
  await urlVariable
    .getByRole("combobox", { name: "Value type", exact: true })
    .click();
  await page.getByRole("option", { name: "Text", exact: true }).click();
  await urlVariable
    .getByRole("combobox", { name: "Value type", exact: true })
    .click();
  await page.getByRole("option", { name: "URL", exact: true }).click();
  await expect(
    drawer
      .getByRole("group", { name: "URL", exact: true })
      .getByRole("checkbox", { name: "Hidden", exact: true }),
  ).toBeChecked();
  await page.setViewportSize({ width: 390, height: 844 });
  await drawer.getByRole("button", { name: "Files", exact: true }).click();
  const replacement = drawer
    .getByRole("group", { name: /^replacement \d+$/ })
    .filter({ has: page.locator('input[value="server.name"]') });
  await replacement.getByLabel("Replacement value", { exact: true }).fill("");
  const configuration = drawer.getByRole("group", {
    name: "configuration file 1",
    exact: true,
  });
  await expect(
    configuration.getByRole("textbox", { name: /^string key/ }),
  ).toHaveCount(0);
  await drawer.getByRole("button", { name: "Ports", exact: true }).click();
  await expect(
    drawer
      .getByRole("group", { name: "port 2", exact: true })
      .getByRole("combobox", { name: "Protocol", exact: true }),
  ).toHaveText("UDP");
  await drawer.getByRole("tab", { name: "JSON / YAML", exact: true }).click();
  const raw = drawer.getByLabel("Definition", { exact: true });
  await expect(
    drawer.getByRole("region", { name: "Compatibility preview", exact: true }),
  ).toBeVisible();
  await expect(
    drawer.getByText("Converted Ember", { exact: true }),
  ).toHaveCount(0);
  const edited = JSON.parse(await raw.inputValue());
  expect(edited.origin).toEqual(original.origin);
  expect(edited.variables[0].validation).toEqual({
    ...original.variables![0]!.validation,
    max: 180,
  });
  expect(edited.configs[0].values["server.name"]).toBe("");
  expect(edited.readiness.any[1].value).toBe("Now listening");
  edited.description = "Pasted JSON and visual fields stay in sync.";
  await raw.fill(JSON.stringify(edited, null, 2));
  await drawer.getByRole("tab", { name: "Visual editor", exact: true }).click();
  await expect(drawer.getByRole("heading", { level: 3 })).toHaveText("Ports");
  await drawer.getByRole("button", { name: "Basics", exact: true }).click();
  await expect(drawer.getByLabel("Description", { exact: true })).toHaveValue(
    edited.description,
  );
  await drawer.getByRole("button", { name: "Review", exact: true }).click();
  const response = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/api/v1/embers/${stored.id}`) &&
      response.request().method() === "PUT",
  );
  await drawer.getByRole("button", { name: "Save Ember", exact: true }).click();
  const saved = await response;
  expect(saved.ok()).toBeTruthy();
  const definition = await saved.json();
  expect(definition.id).toBe(stored.id);
  expect(definition.installer).toEqual(original.installer);
  expect(definition.variables[1]).toEqual(original.variables![1]);
  expect(definition.savedata_paths).toEqual(original.savedata_paths);
  expect(definition.required_ports).toEqual(original.required_ports);
  expect(definition.origin).toEqual(original.origin);
  expect(definition.startup_commands.Debug).toBe(
    "echo Updated; sleep {{SECONDS}}",
  );
  expect(definition.updateUrl).toBe(original.updateUrl);
});

test("pasted invalid JSON stays intact and Pelican YAML can be edited visually with its warnings", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "embers", "Embers");
  await page
    .getByRole("button", { name: "+ Import Ember", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Import Ember",
    exact: true,
  });
  const raw = drawer.getByLabel("Definition", { exact: true });
  await raw.fill('{"schema":1,');
  await drawer.getByRole("tab", { name: "Visual editor", exact: true }).click();
  await expect(raw).toHaveValue('{"schema":1,');
  await expect(
    drawer.getByText(/Your JSON or YAML has been kept/),
  ).toBeVisible();
  const yaml = await readFile(
    new URL(
      "../backend/internal/campfire/service/testdata/eggs/valheim-bepinex.yaml",
      import.meta.url,
    ),
    "utf8",
  );
  await raw.fill(yaml);
  await expect(
    drawer.getByRole("region", { name: "Compatibility preview", exact: true }),
  ).toContainText("Valheim BepInEx fixture");
  await drawer.getByRole("tab", { name: "Visual editor", exact: true }).click();
  await drawer.getByLabel("Name", { exact: true }).fill("Visual Valheim");
  for (const step of [
    "Installation",
    "Runtime",
    "Variables",
    "Ports",
    "Files",
    "Storage",
    "Review",
  ]) {
    await drawer
      .getByRole("button", { name: `Next: ${step}`, exact: true })
      .click();
  }
  const save = drawer.getByRole("button", {
    name: "Import Ember",
    exact: true,
  });
  await expect(save).toBeDisabled();
  await drawer
    .getByRole("checkbox", {
      name: "I understand these setup requirements.",
      exact: true,
    })
    .check();
  await expect(save).toBeEnabled();
  await save.click();
  await expect(drawer).toHaveCount(0);
  const response = await page.request.get("/api/v1/embers");
  const embers = (await response.json()) as ReturnType<typeof completeEmber>[];
  const ember = embers.find((ember) => ember.name === "Visual Valheim");
  expect(ember?.startup).toContain("-backups {{BACKUP_COUNT}}");
  expect(
    ember?.variables?.find((variable) => variable.name === "BEPINEX_URL"),
  ).toMatchObject({ hidden: true, validation: { format: "url" } });
});
