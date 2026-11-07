import type { Page } from "@playwright/test";
import { test, expect, screenshot, prepareScreenshot } from "./fixtures";
import {
  login,
  openScreen,
  openServerActions,
  openServerJobs,
} from "./screens";
import { JobStatus, JobStep, JobType } from "../web/src/contracts/domains";
import { jobFixture } from "./jobFixtures";

interface DestinationSpark {
  id: string;
  name: string;
  connected: boolean;
  disabled: boolean;
}

async function openCopy(
  page: Page,
  action: "Copy" | "Quick copy",
  name = "Survival",
) {
  const menu = await openServerActions(page, name);
  await expect(
    menu.getByRole("menuitem", { name: "Copy jobs", exact: true }),
  ).toHaveCount(0);
  await menu.getByRole("menuitem", { name: action, exact: true }).click();
  const drawer = page.getByRole("dialog", { name: action, exact: true });
  await expect(
    drawer.getByRole("heading", { name: "Jobs", exact: true }),
  ).toHaveCount(0);
  return drawer;
}

test("Copy prefills the creation wizard and creates an independent settings-only server", async ({
  page,
}) => {
  await login(page);
  const drawer = await openCopy(page, "Copy");
  await expect(drawer.getByLabel("Server name", { exact: true })).toHaveValue(
    "Survival (copy)",
  );
  await expect(
    drawer.getByRole("checkbox", { name: /^Copy game files/ }),
  ).not.toBeChecked();
  await screenshot(page, "server-copy-wizard.png", drawer);
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "server-copy-wizard-mobile.png", drawer);
  for (const name of [
    "Next: Variables",
    "Next: Resources & ports",
    "Next: Backups",
    "Next: Access",
    "Next: Review",
  ])
    await drawer.getByRole("button", { name, exact: true }).click();
  const response = page.waitForResponse(
    (response) =>
      response.url().endsWith("/servers/survival/copy") &&
      response.request().method() === "POST",
  );
  await drawer.getByRole("button", { name: "Copy", exact: true }).click();
  const saved = await response;
  expect(saved.ok()).toBeTruthy();
  expect(saved.request().postDataJSON()).toMatchObject({ copy_files: false });
  const server = (await saved.json()) as {
    id: string;
    name: string;
    owner_user_id: number;
  };
  expect(server.id).not.toBe("survival");
  await expect(drawer).toHaveCount(0);
  await expect(
    page.getByRole("tab", { name: "Jobs", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  expect(
    await (
      await page.request.get(`/api/v1/servers/${server.id}/status`)
    ).json(),
  ).toMatchObject({ state: "not_installed" });
  expect(
    await (await page.request.get("/api/v1/servers/survival/status")).json(),
  ).toMatchObject({ state: "running" });
});

test("Quick copy asks only for a Spark and copies a running server's files", async ({
  page,
}) => {
  await login(page);
  const drawer = await openCopy(page, "Quick copy");
  await expect(drawer.getByRole("combobox")).toHaveCount(1);
  await expect(drawer.getByRole("textbox")).toHaveCount(0);
  await prepareScreenshot(page);
  await expect(drawer).toHaveScreenshot([
    "crops",
    "server-quick-copy-dialog.png",
  ]);
  const response = page.waitForResponse(
    (response) =>
      response.url().endsWith("/servers/survival/quick-copy") &&
      response.request().method() === "POST",
  );
  await drawer.getByRole("button", { name: "Quick copy", exact: true }).click();
  const saved = await response;
  expect(saved.ok()).toBeTruthy();
  expect(Object.keys(saved.request().postDataJSON() as object)).toEqual([
    "spark_id",
  ]);
  const server = (await saved.json()) as {
    id: string;
    ports: { host_port: number }[];
  };
  await expect(drawer).toHaveCount(0);
  const history = page.getByRole("region", {
    name: "Jobs",
    exact: true,
  });
  await expect(history.getByText("Complete", { exact: true })).toBeVisible({
    timeout: 20_000,
  });
  expect(
    await (
      await page.request.get(`/api/v1/servers/${server.id}/status`)
    ).json(),
  ).toMatchObject({ state: "stopped", busy: false });
  expect(
    await (await page.request.get("/api/v1/servers/survival/status")).json(),
  ).toMatchObject({ state: "running" });
  const files = await page.request.get(
    `/api/v1/servers/${server.id}/files?path=/world/`,
  );
  expect(files.ok()).toBeTruthy();
  expect(
    ((await files.json()) as { entries: { name: string }[] }).entries.some(
      (entry) => entry.name === "level.dat",
    ),
  ).toBeTruthy();
  await page.reload();
  const details = await openServerJobs(page);
  await expect(details.getByText("Complete", { exact: true })).toBeVisible();
});

for (const action of ["Copy", "Quick copy", "Transfer"] as const) {
  test(`${action} can queue for a disconnected Spark and cancel without connecting`, async ({
    page,
  }) => {
    for (const endpoint of ["sparks", "provisioning"])
      await page.route(`**/api/v1/${endpoint}`, async (route) => {
        const response = await route.fetch();
        const body = (await response.json()) as
          DestinationSpark[] | { sparks: DestinationSpark[] };
        const sparks = Array.isArray(body) ? body : body.sparks;
        const online = sparks.find((spark) => spark.connected);
        if (!online) throw new Error("Missing connected Spark fixture");
        sparks.push({
          ...online,
          id: "disabled-destination",
          name: "Disabled destination",
          disabled: true,
        });
        await route.fulfill({ response, json: body });
      });
    await login(page);
    let drawer;
    if (action === "Transfer") {
      const menu = await openServerActions(page, "Creative");
      await menu.getByRole("menuitem", { name: action, exact: true }).click();
      drawer = page.getByRole("dialog", {
        name: "Transfer Creative",
        exact: true,
      });
    } else drawer = await openCopy(page, action, "Creative");
    if (action === "Copy")
      await drawer.getByRole("checkbox", { name: /^Copy game files/ }).check();
    await drawer
      .getByRole("combobox", {
        name: action === "Copy" ? "Spark" : "Destination Spark",
        exact: true,
      })
      .click();
    await expect(
      page.getByRole("option", { name: "Disabled destination", exact: true }),
    ).toHaveCount(0);
    if (action === "Transfer")
      await expect(
        page.getByRole("option", { name: "Europe", exact: true }),
      ).toHaveCount(0);
    await page
      .getByRole("option", { name: "Offline (Disconnected)", exact: true })
      .click();
    if (action === "Copy")
      for (const name of [
        "Next: Variables",
        "Next: Resources & ports",
        "Next: Backups",
        "Next: Access",
        "Next: Review",
      ])
        await drawer.getByRole("button", { name, exact: true }).click();
    const endpoint = {
      Copy: "copy",
      "Quick copy": "quick-copy",
      Transfer: "transfers",
    }[action];
    const response = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().endsWith(`/servers/creative/${endpoint}`),
    );
    await drawer
      .getByRole("button", {
        name: action === "Transfer" ? "Transfer server" : action,
        exact: true,
      })
      .click();
    expect((await response).ok()).toBeTruthy();
    const jobs = page.getByRole("region", { name: "Jobs", exact: true });
    await expect(jobs.getByText("Waiting", { exact: true })).toBeVisible();
    await expect(
      jobs.getByText("Waiting for Offline to connect.", { exact: true }),
    ).toBeVisible();
    const cancelled = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        /\/jobs\/[^/]+\/cancel$/.test(response.url()),
    );
    await jobs.getByRole("button", { name: "Cancel", exact: true }).click();
    const result = await cancelled;
    expect(result.ok()).toBeTruthy();
    expect(await result.json()).toMatchObject({
      status: JobStatus.Cancelled,
      waiting_for_sparks: [],
    });
  });
}

test("the server Jobs tab shows both copy and transfer progress with retry and cancellation", async ({
  page,
}) => {
  await login(page);
  let job = jobFixture();
  const completed = jobFixture({
    id: "completed-transfer",
    type: JobType.Transfer,
    source_server_id: "survival",
    target_server_id: "survival",
    source_name: "Survival",
    target_name: "Survival",
    status: JobStatus.Completed,
    step: JobStep.Cleanup,
    source_spark_id: "mock-america",
    target_spark_id: "mock-europe",
    source_spark_name: "America",
    target_spark_name: "Europe",
    transferred_bytes: 1024 ** 3,
    can_cancel: false,
  });
  await page.route("**/api/v1/servers/survival/jobs", (route) =>
    route.fulfill({ json: { items: [job, completed], next: "" } }),
  );
  const drawer = await openServerJobs(page);
  const history = drawer.getByRole("row", {
    name: "Copy current-copy",
    exact: true,
  });
  await expect(
    history.getByRole("progressbar", {
      name: "Archive bytes copied",
      exact: true,
    }),
  ).toHaveAttribute("aria-valuetext", "384 MiB / 1 GiB");
  await expect(
    drawer.getByRole("row", {
      name: "Transfer completed-transfer",
      exact: true,
    }),
  ).toContainText("Complete");
  await screenshot(page, "server-jobs.png", drawer);
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "server-jobs-mobile.png", drawer);
  await drawer.getByRole("tab", { name: "Details", exact: true }).click();
  await expect(history).toHaveCount(0);
  await drawer.getByRole("tab", { name: "Jobs", exact: true }).click();
  job = {
    ...job,
    status: JobStatus.Failed,
    can_retry: true,
    error: "Destination Spark disconnected. Retry to copy the retained backup.",
  };
  await expect(history.getByRole("alert")).toHaveText(job.error);
  await page.route("**/api/v1/jobs/current-copy/retry", async (route) => {
    job = { ...job, status: JobStatus.Running, can_retry: false, error: "" };
    await route.fulfill({ json: job });
  });
  await history.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(history.getByRole("alert")).toHaveCount(0);
  await page.route("**/api/v1/jobs/current-copy/cancel", async (route) => {
    job = {
      ...job,
      status: JobStatus.Cancelled,
      can_retry: false,
      can_cancel: false,
    };
    await route.fulfill({ json: job });
  });
  await history.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(history.getByText("Cancelled", { exact: true })).toBeVisible();
  await expect(history.getByRole("progressbar")).toHaveCount(0);
});

test("scoped copy history identifies the other server and the copy direction", async ({
  page,
}) => {
  await login(page);
  const running = jobFixture({
    target_server_id: "modded",
    target_name: "Modded",
  });
  const cancelled = jobFixture({
    ...running,
    id: "cancelled-copy",
    status: JobStatus.Cancelled,
    can_cancel: false,
    finished_at: 1792670400,
  });
  for (const id of ["survival", "modded"])
    await page.route(`**/api/v1/servers/${id}/jobs`, (route) =>
      route.fulfill({ json: { items: [running, cancelled], next: "" } }),
    );
  for (const scope of [
    { name: "Survival", peer: "Modded", peerID: "modded", direction: "→" },
    {
      name: "Modded",
      peer: "Survival",
      peerID: "survival",
      direction: "←",
    },
  ]) {
    const drawer = await openServerJobs(page, scope.name);
    for (const width of [1400, 390]) {
      await page.setViewportSize({ width, height: 900 });
      for (const job of [running, cancelled]) {
        const row = drawer.getByRole("row", {
          name: `Copy ${job.id}`,
          exact: true,
        });
        await expect(row.locator(".server-job-heading")).toHaveText(
          `Copy ${scope.direction} ${scope.peer}`,
        );
        await expect(
          row.getByRole("link", { name: scope.name, exact: true }),
        ).toHaveCount(0);
        const peer = row.getByRole("link", { name: scope.peer, exact: true });
        if (job.status === JobStatus.Cancelled && scope.direction === "→")
          await expect(peer).toHaveCount(0);
        else
          await expect(peer).toHaveAttribute(
            "href",
            `#servers/${scope.peerID}/console`,
          );
      }
    }
    await drawer
      .getByRole("button", { name: "Close", exact: true })
      .first()
      .click();
  }
});

test("copy options open directly into their drawer after loading", async ({
  page,
}) => {
  await login(page);
  for (const action of ["Copy", "Quick copy"]) {
    let release: (() => void) | undefined;
    const ready = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route("**/api/v1/servers/survival/copy", async (route) => {
      await ready;
      await route.continue();
    });
    const menu = await openServerActions(page);
    const loading = page.waitForRequest((request) =>
      request.url().endsWith("/servers/survival/copy"),
    );
    await menu.getByRole("menuitem", { name: action, exact: true }).click();
    await loading;
    await expect(
      page.getByRole("heading", { name: "Survival", level: 1, exact: true }),
    ).toBeVisible();
    await expect(page.locator(".confirm-dialog")).toHaveCount(0);
    await expect(
      page.getByRole("dialog", { name: action, exact: true }),
    ).toHaveCount(0);
    release?.();
    const drawer = page.getByRole("dialog", { name: action, exact: true });
    await expect(drawer).toHaveClass(/drawer/);
    await expect(
      drawer.getByRole("heading", { name: "Jobs", exact: true }),
    ).toHaveCount(0);
    await drawer
      .getByRole("button", { name: "Close", exact: true })
      .first()
      .click();
    await page.unroute("**/api/v1/servers/survival/copy");
  }
});

test("retrying access changes after creation does not create another copy", async ({
  page,
}) => {
  await login(page);
  let failGrant = true;
  let creations = 0;
  page.on("request", (request) => {
    if (
      request.method() === "POST" &&
      request.url().endsWith("/servers/survival/copy")
    )
      creations++;
  });
  await page.route("**/api/v1/servers/*/access/grants", async (route) => {
    if (failGrant)
      await route.fulfill({
        status: 400,
        json: { error: "Grant request could not be saved." },
      });
    else await route.continue();
  });
  const drawer = await openCopy(page, "Copy");
  for (const name of [
    "Next: Variables",
    "Next: Resources & ports",
    "Next: Backups",
    "Next: Access",
  ])
    await drawer.getByRole("button", { name, exact: true }).click();
  await drawer.getByRole("combobox", { name: "User", exact: true }).click();
  await page
    .getByRole("option", { name: "Alex Rivera (alex)", exact: true })
    .click();
  await drawer.getByRole("button", { name: "Add access", exact: true }).click();
  await drawer
    .getByRole("button", { name: "Next: Review", exact: true })
    .click();
  await drawer.getByRole("button", { name: "Copy", exact: true }).click();
  await expect(drawer.getByRole("alert")).toContainText(
    "Grant request could not be saved.",
  );
  expect(creations).toBe(1);
  failGrant = false;
  await drawer.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(drawer).toHaveCount(0);
  await expect(
    page.getByRole("tab", { name: "Jobs", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  expect(creations).toBe(1);
});

test("server viewers do not request job histories they cannot read", async ({
  page,
}) => {
  await login(page, "observer");
  const requests: string[] = [];
  page.on("request", (request) => {
    if (/\/jobs$/.test(request.url())) requests.push(request.url());
  });
  await openScreen(page, "servers", "Servers");
  await page
    .getByRole("button", { name: "Open Survival", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Survival", level: 1, exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("menuitem", { name: "Jobs", exact: true }),
  ).toHaveCount(0);
  expect(requests).toEqual([]);
});

test("the shared Jobs list polls active jobs and caches completed older pages", async ({
  page,
}) => {
  await login(page);
  const jobs = Array.from({ length: 53 }, (_, index) =>
    jobFixture({
      id: `job-${String(53 - index).padStart(3, "0")}`,
      type: index % 2 ? JobType.Transfer : JobType.Copy,
      status: JobStatus.Completed,
      step: JobStep.Cleanup,
      can_cancel: false,
      finished_at: 1792670400,
    }),
  );
  jobs[0] = jobFixture({ id: "active-job" });
  let headRequests = 0;
  let olderRequests = 0;
  await page.route("**/api/v1/servers/survival/jobs*", (route) => {
    const older = new URL(route.request().url()).searchParams.get("before");
    if (older) olderRequests++;
    else headRequests++;
    return route.fulfill({
      json: {
        items: older ? jobs.slice(50) : jobs.slice(0, 50),
        next: older ? "" : "older-page",
      },
    });
  });
  const drawer = await openServerJobs(page);
  const rows = drawer.locator(".server-job-history-item");
  await expect(rows).toHaveCount(50);
  await drawer
    .getByRole("button", { name: "Load older jobs", exact: true })
    .click();
  await expect(rows).toHaveCount(53);
  await expect(
    drawer.getByRole("button", { name: "Load older jobs", exact: true }),
  ).toHaveCount(0);
  await expect.poll(() => headRequests).toBeGreaterThanOrEqual(3);
  expect(olderRequests).toBe(1);
  await expect(rows).toHaveCount(53);
});

test("configuration access can read copy history without platform transfer or Spark access", async ({
  page,
  request,
  baseURL,
}) => {
  await login(page, "viewer");
  const auth = await request.post("/api/v1/auth/login", {
    data: { username: "admin", password: "campfire-e2e-password" },
  });
  expect(auth.ok()).toBeTruthy();
  const { csrf_token } = (await auth.json()) as { csrf_token: string };
  const headers = { "X-CSRF-Token": csrf_token, Origin: baseURL! };
  const roleResponse = await request.post("/api/v1/roles", {
    headers,
    data: {
      name: "Copy history reader",
      scope: "server",
      permissions: ["server.view", "server.configure"],
    },
  });
  expect(roleResponse.ok()).toBeTruthy();
  const role = (await roleResponse.json()) as { id: number };
  expect(
    (
      await request.post("/api/v1/servers/survival/members", {
        headers,
        data: { username: "viewer", role_id: role.id },
      })
    ).ok(),
  ).toBeTruthy();
  const protectedRequests: string[] = [];
  page.on("request", (request) => {
    if (/\/transfers$|\/api\/v1\/sparks(?:\?|$)/.test(request.url()))
      protectedRequests.push(request.url());
  });
  await page.reload();
  const drawer = await openServerJobs(page);
  await expect(
    drawer.getByRole("region", { name: "Jobs", exact: true }),
  ).toContainText("No jobs yet.");
  await expect(drawer.getByRole("row", { name: /^Transfer / })).toHaveCount(0);
  expect(protectedRequests).toEqual([]);
});
