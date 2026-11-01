import type { Page } from "@playwright/test";
import { TransferPhase } from "../web/src/contracts/domains";
import { test, expect, screenshot } from "./fixtures";
import { login, openScreen } from "./screens";

function transferFixture(phase: TransferPhase = TransferPhase.Copying) {
  return {
    id: "current-transfer",
    server_id: "creative",
    source_spark_id: "mock-europe",
    target_spark_id: "mock-america",
    phase,
    active: true,
    can_cancel: true,
    error: "",
    transferred_bytes: 384 * 1024 ** 2,
    total_bytes: 1024 ** 3,
    created_at: 1792670400,
    updated_at: 1792670400,
  };
}

const transfer = transferFixture();

async function openTransfers(page: Page) {
  await openScreen(page, "servers", "Servers");
  await page
    .getByRole("button", { name: "View details for Creative", exact: true })
    .click();
  const details = page.getByRole("dialog", { name: /^Creative/ });
  await details.getByRole("button", { name: "Actions", exact: true }).click();
  await details
    .getByRole("menuitem", { name: "Transfer", exact: true })
    .click();
  return page.getByRole("dialog", { name: "Transfer Creative", exact: true });
}

test("transfer history shows copy progress and cancellation on desktop and mobile", async ({
  page,
}) => {
  await login(page);
  let current = { ...transfer };
  const completed = {
    ...transfer,
    id: "completed-transfer",
    source_spark_id: "mock-america",
    target_spark_id: "mock-europe",
    phase: TransferPhase.Complete,
    active: false,
    can_cancel: false,
    transferred_bytes: transfer.total_bytes,
  };
  await page.route("**/api/v1/servers/creative/transfers", (route) =>
    route.fulfill({ json: [current, completed] }),
  );
  await page.route(
    "**/api/v1/transfers/current-transfer/cancel",
    async (route) => {
      expect(route.request().method()).toBe("POST");
      current = {
        ...current,
        phase: TransferPhase.Cancelled,
        active: false,
        can_cancel: false,
      };
      await route.fulfill({ json: current });
    },
  );
  const drawer = await openTransfers(page);
  const active = drawer.getByRole("region", {
    name: "Transfer current-transfer",
    exact: true,
  });
  const progress = active.getByRole("progressbar", {
    name: "Archive bytes copied",
    exact: true,
  });
  await expect(progress).toHaveAttribute(
    "value",
    String(transfer.transferred_bytes),
  );
  await expect(progress).toHaveAttribute("max", String(transfer.total_bytes));
  await expect(progress).toHaveAttribute("aria-valuetext", "384 MiB / 1 GiB");
  await screenshot(page, "server-transfer-history.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "server-transfer-history-mobile.png");
  await active
    .getByRole("button", { name: "Cancel transfer", exact: true })
    .click();
  await expect(active).toContainText("Cancelled");
  await expect(active.getByRole("progressbar")).toHaveCount(0);
  await expect(active.getByRole("button")).toHaveCount(0);
  await expect(
    drawer.getByRole("region", {
      name: "Transfer completed-transfer",
      exact: true,
    }),
  ).toContainText("Complete");
  await expect(drawer.getByRole("progressbar")).toHaveCount(0);
});

test("failed transfer history allows retry without losing its copy progress", async ({
  page,
}) => {
  await login(page);
  let current = {
    ...transferFixture(TransferPhase.Failed),
    error: "Destination Spark disconnected. Retry to continue copying.",
  };
  await page.route("**/api/v1/servers/creative/transfers", (route) =>
    route.fulfill({ json: [current] }),
  );
  await page.route(
    "**/api/v1/transfers/current-transfer/retry",
    async (route) => {
      expect(route.request().method()).toBe("POST");
      current = { ...current, phase: TransferPhase.Copying, error: "" };
      await route.fulfill({ json: current });
    },
  );
  const drawer = await openTransfers(page);
  const active = drawer.getByRole("region", {
    name: "Transfer current-transfer",
    exact: true,
  });
  await expect(active.getByRole("alert")).toHaveText(current.error);
  await expect(active.getByRole("progressbar")).toHaveCount(0);
  await screenshot(page, "server-transfer-failed.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "server-transfer-failed-mobile.png");
  await active
    .getByRole("button", { name: "Retry transfer", exact: true })
    .click();
  await expect(active).toContainText("Copying archive");
  await expect(active.getByRole("alert")).toHaveCount(0);
  await expect(active.getByRole("progressbar")).toHaveAttribute(
    "value",
    String(transfer.transferred_bytes),
  );
  await expect(
    active.getByRole("button", { name: "Cancel transfer", exact: true }),
  ).toBeEnabled();
});
