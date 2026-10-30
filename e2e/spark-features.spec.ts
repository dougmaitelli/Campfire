import { test, expect, prepareScreenshot } from "./fixtures";
import { login, openScreen } from "./screens";

test("Spark features group related capabilities and report partial support", async ({
  page,
}) => {
  await login(page);
  const capabilities = [
    "server.startup-override.v1",
    "server.game-media.v1",
    "server.game-events.v1",
    "server.game-integration.v1",
    "spark.prune.v1",
    "server.configure.v1",
    "server.lifecycle.v1",
    "server.files.v1",
    "server.files.permissions.v1",
    "server.files.transfer.v1",
    "server.files.archive.v1",
    "server.backups.v1",
    "server.backups.remote.v1",
    "server.metrics.v1",
    "server.console.stream.v1",
    "server.disk-quota.v1",
    "server.transfer.v1",
    "server.reinstall.v1",
    "spark.self-update.v1",
    "spark.self-reinstall.v1",
  ];
  let partial = false;
  await page.route(/\/api\/v1\/sparks(?:\/mock-europe)?$/, async (route) => {
    const response = await route.fetch();
    const value = await response.json();
    const update = (spark: Record<string, unknown>) =>
      spark.id === "mock-europe"
        ? {
            ...spark,
            capabilities: partial
              ? capabilities.filter(
                  (capability) =>
                    ![
                      "server.files.permissions.v1",
                      "spark.self-reinstall.v1",
                      "server.game-events.v1",
                    ].includes(capability),
                )
              : capabilities,
          }
        : spark;
    await route.fulfill({
      response,
      json: Array.isArray(value) ? value.map(update) : update(value),
    });
  });
  await page.reload();
  await openScreen(page, "sparks", "Sparks");
  const details = page.getByRole("button", {
    name: "View details for Europe",
    exact: true,
  });
  await details.click();
  const dialog = page.getByRole("dialog");
  const features = dialog.getByRole("region", {
    name: "Spark features",
    exact: true,
  });
  await expect(
    features.getByRole("heading", { name: "Features (17/17)", exact: true }),
  ).toBeVisible();
  await expect(features.getByRole("listitem")).toHaveCount(17);
  await page.setViewportSize({ width: 1400, height: 1400 });
  await features.scrollIntoViewIfNeeded();
  await prepareScreenshot(page);
  await expect(features).toHaveScreenshot("spark-features.png");
  await page.setViewportSize({ width: 390, height: 1800 });
  await prepareScreenshot(page);
  await expect(features).toHaveScreenshot("spark-features-mobile.png");
  await dialog
    .getByRole("button", { name: "Close", exact: true })
    .first()
    .click();

  partial = true;
  await page.setViewportSize({ width: 1400, height: 1400 });
  await page.reload();
  await details.click();
  await expect(
    features.getByRole("heading", { name: "Features (14/17)", exact: true }),
  ).toBeVisible();
  for (const label of [
    "File management",
    "Remote Spark updates",
    "Server state and event publishing",
  ]) {
    await expect(
      features.getByRole("listitem").filter({ hasText: label }),
    ).toContainText("Partial support");
  }
  await features.scrollIntoViewIfNeeded();
  await prepareScreenshot(page);
  await expect(features).toHaveScreenshot("spark-features-partial.png");
});
