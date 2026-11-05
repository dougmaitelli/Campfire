import { test, expect, screenshot, prepareScreenshot } from "./fixtures";
import { login, openScreen } from "./screens";
import { jobFixture } from "./jobFixtures";
import { JobStatus, JobStep, JobType } from "../web/src/contracts/domains";

test("admins can find, filter and manage jobs across servers while drawers stay scoped", async ({
  page,
}) => {
  let failed = jobFixture({
    id: "failed-copy",
    status: JobStatus.Failed,
    error: "Destination storage is full.",
    can_retry: true,
    source_server_id: "creative",
    source_name: "Creative",
    target_server_id: "modded",
    target_name: "Modded",
  });
  const waiting = jobFixture({
    id: "waiting-copy",
    status: JobStatus.Pending,
    step: JobStep.Preparing,
    transferred_bytes: 0,
    total_bytes: 0,
    target_spark_id: "mock-offline",
    target_spark_name: "Offline",
    waiting_for_sparks: ["mock-offline"],
  });
  const running = jobFixture({
    id: "running-transfer",
    type: JobType.Transfer,
    source_server_id: "creative",
    target_server_id: "creative",
    source_name: "Creative",
    target_name: "Creative",
  });
  const completed = jobFixture({
    id: "completed-copy",
    status: JobStatus.Completed,
    step: JobStep.Cleanup,
    target_server_id: "modded",
    target_name: "Modded",
    can_cancel: false,
    finished_at: 1792670400,
  });
  const older = jobFixture({
    id: "older-cancelled-copy",
    status: JobStatus.Cancelled,
    can_cancel: false,
  });
  await page.route(/\/api\/v1\/jobs(?:\?.*)?$/, async (route) => {
    const query = new URL(route.request().url()).searchParams;
    const items = query.has("before")
      ? [older]
      : [waiting, running, completed, failed].filter(
          (job) =>
            (!query.get("type") || job.type === query.get("type")) &&
            (!query.get("status") || job.status === query.get("status")),
        );
    await route.fulfill({
      json: {
        items,
        next: query.size === 0 ? "older-page" : "",
      },
    });
  });
  await page.route("**/api/v1/jobs/failed-copy/retry", async (route) => {
    failed = {
      ...failed,
      status: JobStatus.Running,
      can_retry: false,
      error: "",
    };
    await route.fulfill({ json: failed });
  });
  await page.route("**/api/v1/jobs/failed-copy/cancel", async (route) => {
    failed = { ...failed, status: JobStatus.Cancelled, can_cancel: false };
    await route.fulfill({ json: failed });
  });
  await login(page);
  await openScreen(page, "jobs", "Jobs");
  const jobs = page.getByRole("region", { name: "Jobs", exact: true });
  await expect(jobs.getByText("Waiting for Offline to connect.")).toBeVisible();
  await expect(jobs.locator("tbody tr")).toHaveCount(4);
  await expect(
    jobs.getByRole("row", { name: "Copy failed-copy", exact: true }),
  ).toBeInViewport();
  await screenshot(page, "jobs.png");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBeTruthy();
  await prepareScreenshot(page);
  await expect(page).toHaveScreenshot("jobs-page-mobile.png", {
    fullPage: true,
  });
  await page.setViewportSize({ width: 1400, height: 900 });
  await jobs
    .getByRole("button", { name: "Load older jobs", exact: true })
    .click();
  await expect(jobs.locator("tbody tr")).toHaveCount(5);
  await page.getByRole("combobox", { name: "Status", exact: true }).click();
  await page.getByRole("option", { name: "Complete", exact: true }).click();
  await expect(jobs.locator("tbody tr")).toHaveCount(1);
  await page.getByRole("combobox", { name: "Job type", exact: true }).click();
  await page.getByRole("option", { name: "Transfer", exact: true }).click();
  await expect(jobs.getByText("No jobs yet.", { exact: true })).toBeVisible();
  await page.getByRole("combobox", { name: "Job type", exact: true }).click();
  await page.getByRole("option", { name: "All types", exact: true }).click();
  await page.getByRole("combobox", { name: "Status", exact: true }).click();
  await page.getByRole("option", { name: "All statuses", exact: true }).click();
  const failure = jobs.getByRole("row", {
    name: "Copy failed-copy",
    exact: true,
  });
  await failure.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(failure.getByText("Running", { exact: true })).toBeVisible();
  await failure.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(failure.getByText("Cancelled", { exact: true })).toBeVisible();
  const scoped = page.waitForRequest("**/api/v1/servers/creative/jobs");
  await jobs
    .getByRole("row", { name: "Transfer running-transfer", exact: true })
    .getByRole("link", { name: "Creative", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Server", exact: true });
  await drawer.getByRole("tab", { name: "Jobs", exact: true }).click();
  await scoped;
  await expect(page).toHaveURL(/#jobs$/);
  await expect(drawer.getByText("No jobs yet.", { exact: true })).toBeVisible();
});

test("non-admin users cannot open platform jobs or request their history", async ({
  page,
}) => {
  await login(page, "sam");
  await expect(
    page
      .getByRole("navigation", { name: "Main navigation", exact: true })
      .getByRole("link", { name: "Jobs", exact: true }),
  ).toHaveCount(0);
  let requests = 0;
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/api/v1/jobs") requests++;
  });
  await page.goto("/#jobs");
  await expect(
    page.getByRole("heading", { name: "You do not have access to this page" }),
  ).toBeVisible();
  expect(requests).toBe(0);
  expect((await page.request.get("/api/v1/jobs")).status()).toBe(403);
});

test("completed-job filters refresh older pages when another job finishes", async ({
  page,
}) => {
  await page.clock.install();
  const newest = jobFixture({
    id: "newest-completed",
    status: JobStatus.Completed,
    can_cancel: false,
  });
  const oldest = jobFixture({
    id: "oldest-completed",
    status: JobStatus.Completed,
    can_cancel: false,
  });
  const finished = jobFixture({
    id: "recently-completed-older-job",
    status: JobStatus.Completed,
    can_cancel: false,
  });
  let completed = false;
  await page.route(/\/api\/v1\/jobs(?:\?.*)?$/, (route) => {
    const older = new URL(route.request().url()).searchParams.has("before");
    return route.fulfill({
      json: {
        items: older ? (completed ? [finished, oldest] : [oldest]) : [newest],
        next: older ? "" : "older-page",
      },
    });
  });
  await login(page);
  await openScreen(page, "jobs", "Jobs");
  await page.getByRole("combobox", { name: "Status", exact: true }).click();
  await page.getByRole("option", { name: "Complete", exact: true }).click();
  await page
    .getByRole("button", { name: "Load older jobs", exact: true })
    .click();
  await expect(
    page.getByRole("row", { name: "Copy oldest-completed", exact: true }),
  ).toBeVisible();
  completed = true;
  await page.clock.runFor(30_000);
  await expect(
    page.getByRole("row", {
      name: "Copy recently-completed-older-job",
      exact: true,
    }),
  ).toBeVisible();
});
