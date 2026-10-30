import { readFile } from "node:fs/promises";
import { test, expect, screenshot } from "./fixtures";
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
});
