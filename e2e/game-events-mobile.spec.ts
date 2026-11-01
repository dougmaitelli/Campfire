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

test("native activity date fields remain readable empty and filled on mobile", async ({
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
  const feed = page.getByRole("region", { name: "Game activity", exact: true });
  await expect(
    feed.getByText("Alex joined the server", { exact: true }),
  ).toBeVisible();
  const filters = feed.locator("form");
  await prepareScreenshot(page);
  await expect(filters).toHaveScreenshot("activity-date-filters-mobile.png");

  await feed
    .getByLabel("Occurred after", { exact: true })
    .fill("2030-01-01T10:30");
  await feed
    .getByLabel("Occurred before", { exact: true })
    .fill("2030-01-02T14:45");
  await feed
    .getByRole("heading", { name: "Game activity", exact: true })
    .click();
  await prepareScreenshot(page);
  await expect(filters).toHaveScreenshot(
    "activity-date-filters-filled-mobile.png",
  );
});
