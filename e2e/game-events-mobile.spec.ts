import { test, expect, prepareScreenshot } from "./fixtures";

// Android uses picker-only date controls instead of segmented desktop fields.
test.use({
  viewport: { width: 390, height: 844 },
  isMobile: true,
  hasTouch: true,
  launchOptions: {
    args: [
      "--disable-blink-features=InputMultipleFieldsUI,InputMultipleFieldsUIWithPointerChecks",
    ],
  },
});

test("native activity date fields filter events on mobile", async ({
  page,
}) => {
  expect(
    (
      await page.request.post("/api/v1/auth/login", {
        data: { username: "admin", password: "campfire-e2e-password" },
      })
    ).ok(),
  ).toBeTruthy();
  await page.goto("/#servers/survival/activity");
  const feed = page.getByRole("region", { name: "Activity", exact: true });
  await expect(
    feed.getByText("Alex joined the server", { exact: true }),
  ).toBeVisible();
  const filters = feed.locator("form");
  await prepareScreenshot(page);
  await expect(filters).toHaveScreenshot([
    "crops",
    "activity-date-filters-mobile.png",
  ]);

  await feed
    .getByLabel("Recorded after", { exact: true })
    .fill("2030-01-01T10:30");
  await feed
    .getByLabel("Recorded before", { exact: true })
    .fill("2030-01-02T14:45");
  await feed.getByRole("heading", { name: "Activity", exact: true }).click();
  await prepareScreenshot(page);
  await expect(filters).toHaveScreenshot([
    "crops",
    "activity-date-filters-filled-mobile.png",
  ]);
  await feed
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(
    feed.getByRole("heading", {
      name: "No activity matches these filters.",
      exact: true,
    }),
  ).toBeVisible();
  await feed
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await expect(feed.getByLabel("Recorded after", { exact: true })).toHaveValue(
    "",
  );
  await expect(feed.getByLabel("Recorded before", { exact: true })).toHaveValue(
    "",
  );
  await expect(
    feed.getByText("Alex joined the server", { exact: true }),
  ).toBeVisible();
});
