import { test, expect, screenshot } from "./fixtures";
import { login, openScreen } from "./screens";

test("Spark names can be edited without changing identity or server placement", async ({
  page,
}) => {
  await login(page);
  const originalResponse = await page.request.get("/api/v1/sparks/mock-europe");
  expect(originalResponse.ok()).toBeTruthy();
  const original = (await originalResponse.json()) as {
    id: string;
    created_at: string;
    connected: boolean;
  };
  await openScreen(page, "sparks", "Sparks");
  await page
    .getByRole("button", { name: "View details for Europe", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Edit Spark", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "Edit Spark", exact: true });
  const name = dialog.getByRole("textbox", { name: "Name", exact: true });
  await expect(name).toBeEditable();
  await name.fill("   ");
  await expect(
    dialog.getByRole("button", {
      name: "Next: Resources & networking",
      exact: true,
    }),
  ).toBeDisabled();
  await name.fill("North node");
  await screenshot(page, "spark-edit-name.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "spark-edit-name-mobile.png");

  await name.fill("america");
  await dialog.getByRole("button", { name: "Review", exact: true }).click();
  await dialog
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toContainText(
    "A Spark with that name already exists",
  );
  await dialog.getByRole("button", { name: "General", exact: true }).click();
  await name.fill("  North node  ");
  await dialog.getByRole("button", { name: "Review", exact: true }).click();
  await dialog
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("button", {
      name: "View details for North node",
      exact: true,
    }),
  ).toBeVisible();

  const renamedResponse = await page.request.get(
    `/api/v1/sparks/${original.id}`,
  );
  expect(renamedResponse.ok()).toBeTruthy();
  expect(await renamedResponse.json()).toMatchObject({
    id: original.id,
    name: "North node",
    created_at: original.created_at,
    connected: original.connected,
  });
  const serverResponse = await page.request.get("/api/v1/servers/survival");
  expect(serverResponse.ok()).toBeTruthy();
  expect(await serverResponse.json()).toMatchObject({
    spark_id: original.id,
    spark_name: "North node",
  });
  await page.reload();
  await page
    .getByRole("button", { name: "View details for North node", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Edit Spark", exact: true })
    .click();
  await expect(name).toHaveValue("North node");
});
