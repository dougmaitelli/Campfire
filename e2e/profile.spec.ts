import { test, expect } from "./fixtures";
import { login } from "./screens";

test("Escape from password MFA verification preserves the profile draft", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/v1/auth/mfa", (route) =>
    route.fulfill({
      json: {
        enabled: true,
        available: true,
        recovery_remaining: 10,
        devices: [{ id: "phone", name: "Phone", created_at: 1700000000 }],
      },
    }),
  );
  await page
    .getByRole("button", { name: "Account options for Developer", exact: true })
    .click();
  await page.getByRole("button", { name: "My profile", exact: true }).click();
  await page
    .getByLabel("Name (optional)", { exact: true })
    .fill("Unsaved draft");
  await page
    .getByLabel("New password (optional)", { exact: true })
    .fill("Review-only-password1!");
  await page
    .getByLabel("Repeat password", { exact: true })
    .fill("Review-only-password1!");
  await page
    .getByLabel("Current password", { exact: true })
    .fill("not submitted");
  await page.getByRole("button", { name: "Save profile", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "Verify password change", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("dialog", { name: "Verify password change", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("dialog", { name: "My profile", exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("Name (optional)", { exact: true })).toHaveValue(
    "Unsaved draft",
  );
  await expect(
    page.getByLabel("New password (optional)", { exact: true }),
  ).toHaveValue("Review-only-password1!");
  await expect(page.getByLabel("Repeat password", { exact: true })).toHaveValue(
    "Review-only-password1!",
  );
});
