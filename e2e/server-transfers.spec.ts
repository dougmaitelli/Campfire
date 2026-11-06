import { JobStatus, JobStep, JobType } from "../web/src/contracts/domains";
import { jobFixture } from "./jobFixtures";
import { test, expect, screenshot } from "./fixtures";
import { login, openScreen, openServerJobs } from "./screens";

function transferFixture(status: JobStatus = JobStatus.Running) {
  return jobFixture({
    id: "current-transfer",
    type: JobType.Transfer,
    status,
    step: JobStep.Copying,
    source_server_id: "creative",
    target_server_id: "creative",
    source_name: "Creative",
    target_name: "Creative",
    can_retry: status === JobStatus.Failed,
  });
}
const transfer = transferFixture();

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
    source_spark_name: "America",
    target_spark_name: "Europe",
    status: JobStatus.Completed,
    can_cancel: false,
    transferred_bytes: transfer.total_bytes,
  };
  await page.route("**/api/v1/servers/creative/jobs", (route) =>
    route.fulfill({ json: { items: [current, completed], next: "" } }),
  );
  await page.route("**/api/v1/jobs/current-transfer/cancel", async (route) => {
    expect(route.request().method()).toBe("POST");
    current = {
      ...current,
      status: JobStatus.Cancelled,
      can_cancel: false,
    };
    await route.fulfill({ json: current });
  });
  const drawer = await openServerJobs(page, "Creative");
  const active = drawer.getByRole("row", {
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
  await screenshot(page, "server-transfer-history.png", drawer);
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "server-transfer-history-mobile.png", drawer);
  await active.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(active).toContainText("Cancelled");
  await expect(active.getByRole("progressbar")).toHaveCount(0);
  await expect(active.getByRole("button")).toHaveCount(0);
  await expect(
    drawer.getByRole("row", {
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
    ...transferFixture(JobStatus.Failed),
    error: "Destination Spark disconnected. Retry to continue copying.",
  };
  await page.route("**/api/v1/servers/creative/jobs", (route) =>
    route.fulfill({ json: { items: [current], next: "" } }),
  );
  await page.route("**/api/v1/jobs/current-transfer/retry", async (route) => {
    expect(route.request().method()).toBe("POST");
    current = {
      ...current,
      status: JobStatus.Running,
      can_retry: false,
      error: "",
    };
    await route.fulfill({ json: current });
  });
  const drawer = await openServerJobs(page, "Creative");
  const active = drawer.getByRole("row", {
    name: "Transfer current-transfer",
    exact: true,
  });
  await expect(active.getByRole("alert")).toHaveText(current.error);
  await expect(active.getByRole("progressbar")).toHaveCount(0);
  await screenshot(page, "server-transfer-failed.png", drawer);
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "server-transfer-failed-mobile.png", drawer);
  await active.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(active).toContainText("Copying files");
  await expect(active.getByRole("alert")).toHaveCount(0);
  await expect(active.getByRole("progressbar")).toHaveAttribute(
    "value",
    String(transfer.transferred_bytes),
  );
  await expect(
    active.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeEnabled();
});

test("starting a transfer returns to the server Jobs tab", async ({ page }) => {
  await login(page);
  let started = false;
  await page.route("**/api/v1/servers/creative/transfers", async (route) => {
    expect(route.request().method()).toBe("POST");
    expect(route.request().postDataJSON()).toMatchObject({
      target_spark_id: "mock-america",
    });
    started = true;
    await route.fulfill({
      status: 202,
      json: transferFixture(JobStatus.Pending),
    });
  });
  await page.route("**/api/v1/servers/creative/jobs", (route) =>
    route.fulfill({
      json: {
        items: started ? [transferFixture(JobStatus.Pending)] : [],
        next: "",
      },
    }),
  );
  await openScreen(page, "servers", "Servers");
  await page
    .getByRole("button", { name: "View details for Creative", exact: true })
    .click();
  const details = page.getByRole("dialog", { name: /^Creative/ });
  await details.getByRole("button", { name: "Actions", exact: true }).click();
  await details
    .getByRole("menuitem", { name: "Transfer", exact: true })
    .click();
  const transferDrawer = page.getByRole("dialog", {
    name: "Transfer Creative",
    exact: true,
  });
  await expect(
    transferDrawer.getByRole("heading", {
      name: "Transfer history",
      exact: true,
    }),
  ).toHaveCount(0);
  await transferDrawer
    .getByRole("combobox", { name: "Destination Spark", exact: true })
    .click();
  await page.getByRole("option", { name: "America", exact: true }).click();
  await transferDrawer
    .getByRole("button", { name: "Transfer server", exact: true })
    .click();
  await expect(transferDrawer).toHaveCount(0);
  await expect(
    details.getByRole("tab", { name: "Jobs", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(
    details.getByRole("region", { name: "Jobs", exact: true }),
  ).toContainText("Pending");
});
