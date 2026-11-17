import { test, expect, screenshot, pageFixture } from "./fixtures";
import { login, openScreen } from "./screens";
import { jobFixture } from "./jobFixtures";
import { JobStatus, JobStep, JobType } from "../web/src/contracts/domains";

test("admins can manage global jobs and open server Activity from resource details", async ({
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
    const items =
      query.get("page") === "2"
        ? [older]
        : [waiting, running, completed, failed].filter(
            (job) =>
              (!query.get("type") || job.type === query.get("type")) &&
              (!query.get("status") || job.status === query.get("status")),
          );
    await route.fulfill({
      json: pageFixture(
        items,
        Number(query.get("page") ?? 1),
        query.has("type") || query.has("status") ? items.length : 21,
      ),
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
  const summary = jobs.locator(".pagination-summary");
  const summaryRightGap = () =>
    summary.evaluate((element) =>
      Math.abs(
        element.closest(".pagination")!.getBoundingClientRect().right -
          element.getBoundingClientRect().right,
      ),
    );
  await expect(summary).toHaveText("Showing 1–20 of 21");
  await expect.poll(summaryRightGap).toBeLessThanOrEqual(1);
  await screenshot(page, "jobs.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect.poll(summaryRightGap).toBeLessThanOrEqual(1);
  await screenshot(page, "jobs-mobile.png");
  await page.setViewportSize({ width: 1400, height: 900 });
  await jobs.getByRole("button", { name: "Next", exact: true }).click();
  await expect(jobs.locator("tbody tr")).toHaveCount(1);
  await expect(
    jobs.getByRole("row", { name: "Copy older-cancelled-copy", exact: true }),
  ).toBeVisible();
  await expect(summary).toHaveText("Showing 21–21 of 21");
  await page.getByRole("combobox", { name: "Status", exact: true }).click();
  await page.getByRole("option", { name: "Complete", exact: true }).click();
  await expect(jobs.locator("tbody tr")).toHaveCount(1);
  await page.getByRole("combobox", { name: "Job type", exact: true }).click();
  await page.getByRole("option", { name: "Transfer", exact: true }).click();
  const empty = jobs.getByRole("status");
  await expect(
    empty.getByRole("heading", { name: "No jobs yet.", exact: true }),
  ).toBeVisible();
  await expect(jobs.getByRole("table")).toHaveCount(0);
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
  await jobs
    .getByRole("row", { name: "Transfer running-transfer", exact: true })
    .getByRole("link", { name: "Creative", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Server", exact: true });
  await expect(
    drawer.getByRole("tab", { name: "Jobs", exact: true }),
  ).toHaveCount(0);
  await expect(page).toHaveURL(/#jobs$/);
  await drawer
    .getByRole("link", { name: "Open console for Creative", exact: true })
    .click();
  await expect(page).toHaveURL(/#servers\/creative\/console$/);
  await page.getByRole("link", { name: "Activity", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Activity", exact: true }),
  ).toBeVisible();
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
    const older =
      new URL(route.request().url()).searchParams.get("page") === "2";
    return route.fulfill({
      json: pageFixture(
        older ? (completed ? [finished, oldest] : [oldest]) : [newest],
        older ? 2 : 1,
        52,
      ),
    });
  });
  await login(page);
  await openScreen(page, "jobs", "Jobs");
  await page.getByRole("combobox", { name: "Status", exact: true }).click();
  await page.getByRole("option", { name: "Complete", exact: true }).click();
  await page.getByRole("button", { name: "Next", exact: true }).click();
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
