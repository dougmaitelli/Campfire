import { readFile } from "node:fs/promises";
import { test, expect, screenshot, prepareScreenshot } from "./fixtures";
import { login, openScreen } from "./screens";

test("Pelican Valheim BepInEx YAML previews and imports without losing URL constraints or game backup variables", async ({
  page,
}) => {
  const definition = await readFile(
    new URL(
      "../backend/internal/campfire/service/testdata/eggs/valheim-bepinex.yaml",
      import.meta.url,
    ),
    "utf8",
  );
  await login(page);
  await openScreen(page, "embers", "Embers");
  await page
    .getByRole("button", { name: "+ Import Ember", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Import Ember",
    exact: true,
  });
  await screenshot(page, "ember-import-sources.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "ember-import-sources-mobile.png");
  await page.setViewportSize({ width: 1400, height: 900 });
  await drawer
    .getByRole("textbox", { name: "Definition", exact: true })
    .fill(definition);
  const preview = drawer.getByRole("region", {
    name: "Compatibility preview",
    exact: true,
  });
  await expect(preview.getByRole("heading")).toContainText(
    "Valheim BepInEx fixture",
  );
  await expect(drawer.getByRole("alert")).toHaveCount(0);
  const importButton = drawer.getByRole("button", {
    name: "Import Ember",
    exact: true,
  });
  await expect(importButton).toBeDisabled();
  await drawer
    .getByRole("checkbox", {
      name: "I understand these setup requirements.",
      exact: true,
    })
    .check();
  await expect(importButton).toBeEnabled();
  await screenshot(page, "pelican-valheim-import.png");
  await importButton.click();
  await expect(drawer).toHaveCount(0);
  const response = await page.request.get("/api/v1/embers");
  const embers = (await response.json()) as {
    id: string;
    name: string;
    startup_commands: Record<string, string>;
    variables: {
      name: string;
      default: string;
      hidden?: boolean;
      validation?: { format?: string };
    }[];
  }[];
  const imported = embers.find(
    (ember) => ember.name === "Valheim BepInEx fixture",
  );
  expect(imported).toBeDefined();
  expect(
    imported?.variables.find((variable) => variable.name === "BEPINEX_URL"),
  ).toMatchObject({ hidden: true, validation: { format: "url" } });
  expect(
    imported?.variables.find((variable) => variable.name === "BACKUP_COUNT")
      ?.default,
  ).toBe("4");
  expect(imported?.startup_commands.Default).toContain(
    "-backups {{BACKUP_COUNT}}",
  );
  await openScreen(page, "servers", "Servers");
  await page
    .getByRole("button", { name: "+ Create server", exact: true })
    .click();
  const create = page.getByRole("dialog", {
    name: "Create server",
    exact: true,
  });
  await create.getByRole("combobox", { name: "Ember", exact: true }).click();
  await page
    .getByRole("option", { name: /^Valheim BepInEx fixture ·/ })
    .click();
  const crossplay = create.getByRole("switch", {
    name: "ENABLE_CROSSPLAY",
    exact: true,
  });
  await expect(crossplay).not.toBeChecked();
  await expect(
    create.getByRole("switch", { name: "AUTO_UPDATE", exact: true }),
  ).toBeChecked();
  await expect(
    create.getByRole("textbox", { name: "ENABLE_CROSSPLAY", exact: true }),
  ).toHaveCount(0);
  await expect(
    create.getByRole("textbox", { name: "BACKUP_COUNT", exact: true }),
  ).toHaveValue("4");
  await expect(
    create.getByRole("textbox", { name: "SRCDS_APPID", exact: true }),
  ).toHaveAttribute("readonly", "");
  await expect(create.getByLabel("BEPINEX_URL", { exact: true })).toHaveCount(
    0,
  );
  await crossplay.check();
  await create
    .getByRole("switch", { name: "PUBLIC_SERVER", exact: true })
    .uncheck();
  const fields = create.locator(".server-variable-fields");
  await prepareScreenshot(page);
  await expect(fields).toHaveScreenshot("egg-boolean-variables.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await crossplay.evaluate((control) =>
    control.scrollIntoView({ block: "center" }),
  );
  await screenshot(page, "egg-boolean-variables-mobile.png");
  await page.setViewportSize({ width: 1400, height: 900 });
  for (const label of [
    "Next: Resources & ports",
    "Next: Backups",
    "Next: Access",
    "Next: Review",
  ]) {
    await create.getByRole("button", { name: label, exact: true }).click();
  }
  await expect(
    create
      .locator(".details-list > div")
      .filter({ has: page.getByText("ENABLE_CROSSPLAY", { exact: true }) }),
  ).toContainText("Enabled");
  await expect(
    create
      .locator(".details-list > div")
      .filter({ has: page.getByText("PUBLIC_SERVER", { exact: true }) }),
  ).toContainText("Disabled");
  await create
    .getByRole("button", { name: "Create server", exact: true })
    .click();
  await expect(create).toHaveCount(0);
  const serversResponse = await page.request.get("/api/v1/servers");
  const servers = (await serversResponse.json()) as {
    id: string;
    ember_id: string;
    name: string;
    variables: Record<string, string>;
  }[];
  const server = servers.find((entry) => entry.ember_id === imported!.id);
  expect(server?.variables.ENABLE_CROSSPLAY).toBe("1");
  expect(server?.variables.PUBLIC_SERVER).toBe("0");
  await page
    .getByRole("button", {
      name: `View details for ${server!.name}`,
      exact: true,
    })
    .click();
  await expect(
    page
      .getByRole("dialog")
      .locator(".details-list > div")
      .filter({ has: page.getByText("ENABLE_CROSSPLAY", { exact: true }) }),
  ).toContainText("Enabled");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Edit server", exact: true })
    .click();
  const edit = page.getByRole("dialog", { name: "Edit server", exact: true });
  const savedCrossplay = edit.getByRole("switch", {
    name: "ENABLE_CROSSPLAY",
    exact: true,
  });
  await expect(savedCrossplay).toBeChecked();
  await savedCrossplay.uncheck();
  for (const label of [
    "Next: Resources & ports",
    "Next: Backups",
    "Next: Access",
    "Next: Review",
  ]) {
    await edit.getByRole("button", { name: label, exact: true }).click();
  }
  await edit.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(edit).toHaveCount(0);
  const saved = await page.request.get(`/api/v1/servers/${server!.id}`);
  expect(
    ((await saved.json()) as { variables: Record<string, string> }).variables
      .ENABLE_CROSSPLAY,
  ).toBe("0");
});
