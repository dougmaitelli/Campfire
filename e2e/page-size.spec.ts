import { test, expect, pageFixture } from "./fixtures";
import { login } from "./screens";
import { jobFixture } from "./jobFixtures";
import { JobStatus, JobStep } from "../web/src/contracts/domains";

test("page sizes reset the page, replace rows, preserve filters and history, and poll only the selected size", async ({
  page,
}) => {
  await page.clock.install();
  await login(page);
  const jobs = Array.from({ length: 65 }, (_, index) =>
    jobFixture({
      id: `sized-job-${index}`,
      status: JobStatus.Completed,
      step: JobStep.Cleanup,
      can_cancel: false,
      finished_at: 1792670400,
    }),
  );
  const requests: URLSearchParams[] = [];
  await page.route("**/api/v1/jobs?*", (route) => {
    const query = new URL(route.request().url()).searchParams;
    requests.push(query);
    const number = Number(query.get("page") ?? 1);
    const size = Number(query.get("page_size") ?? 20);
    return route.fulfill({
      json: pageFixture(
        jobs.slice((number - 1) * size, number * size),
        number,
        jobs.length,
        size,
      ),
    });
  });
  await page.goto("/#jobs?status=completed&page=2");
  const list = page.getByRole("region", { name: "Jobs", exact: true });
  const size = list.getByRole("combobox", {
    name: "Rows per page",
    exact: true,
  });
  const rows = list.locator("tbody tr");
  await expect(rows).toHaveCount(20);
  await expect(list.getByRole("status")).toHaveText("Showing 21–40 of 65");
  await size.click();
  await page.getByRole("option", { name: "50", exact: true }).click();
  await expect(rows).toHaveCount(50);
  await expect(page).toHaveURL(/#jobs\?status=completed&page_size=50$/);
  requests.length = 0;
  await page.clock.runFor(35000);
  await expect.poll(() => requests.length).toBeGreaterThan(0);
  expect(
    requests.every(
      (query) =>
        query.get("page_size") === "50" &&
        query.get("page") === "1" &&
        query.get("status") === "completed",
    ),
  ).toBe(true);
  await list.getByRole("button", { name: "Next", exact: true }).click();
  await expect(rows).toHaveCount(15);
  await expect(list.getByRole("status")).toHaveText("Showing 51–65 of 65");
  await size.click();
  await page.getByRole("option", { name: "100", exact: true }).click();
  await expect(rows).toHaveCount(65);
  await expect(size).toHaveText("100");
  await page.goBack();
  await expect(rows).toHaveCount(15);
  await expect(size).toHaveText("50");
  await page.goForward();
  await expect(rows).toHaveCount(65);
  await expect(size).toHaveText("100");
  await size.click();
  await page.getByRole("option", { name: "20", exact: true }).click();
  await expect(rows).toHaveCount(20);
  await expect(page).toHaveURL(/#jobs\?status=completed$/);
});
