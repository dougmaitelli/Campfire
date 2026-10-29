import { test, expect, screenshot } from "./fixtures";
import { login, openScreen } from "./screens";

test("upcoming schedule rows open details while related links and toggles stay independent", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "dashboard", "Overview");
  const table = page.getByRole("table", {
    name: "Upcoming schedules",
    exact: true,
  });
  const row = table.getByRole("row").filter({ hasText: "Nightly restart" });
  await expect(row).toBeVisible();
  await expect(
    row.getByRole("link", { name: "Nightly restart", exact: true }),
  ).toHaveCount(0);
  await expect(table).toHaveScreenshot("upcoming-schedules.png");
  await expect(page.locator(".dashboard-activity-grid")).toHaveScreenshot(
    "dashboard-activity.png",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator(".dashboard-activity-grid")).toHaveScreenshot(
    "dashboard-activity-mobile.png",
  );
  await page.setViewportSize({ width: 1400, height: 900 });
  await row.getByRole("link", { name: "Survival", exact: true }).click();
  const server = page.getByRole("dialog", { name: "Server", exact: true });
  await expect(server).toBeVisible();
  await expect(
    page.getByRole("dialog", { name: "Nightly restart", exact: true }),
  ).toHaveCount(0);
  await server
    .getByRole("button", { name: "Close", exact: true })
    .last()
    .click();
  await row.getByRole("cell").nth(2).click();
  const schedule = page.getByRole("dialog", {
    name: "Nightly restart",
    exact: true,
  });
  await expect(schedule).toBeVisible();
  await expect(page).toHaveURL(/#dashboard$/);
  await schedule
    .getByRole("button", { name: "Close", exact: true })
    .last()
    .click();
  await row.getByRole("switch").click();
  await expect(row).toHaveCount(0);
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("schedules open an overview before editing and saved changes persist", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "schedules", "Schedules");
  await page
    .getByRole("button", {
      name: "View details for Nightly restart",
      exact: true,
    })
    .click();
  const overview = page.getByRole("dialog", {
    name: "Nightly restart",
    exact: true,
  });
  await expect(overview.getByText("0 3 * * *", { exact: true })).toBeVisible();
  await expect(overview.getByText("UTC", { exact: true })).toBeVisible();
  await expect(overview.getByRole("textbox")).toHaveCount(0);
  const history = overview.getByRole("region", {
    name: "Execution history",
    exact: true,
  });
  await expect(history.getByRole("row")).toHaveCount(2);
  await expect(history.getByText("Succeeded", { exact: true })).toBeVisible();
  await expect(history).toHaveScreenshot("schedule-execution-history.png");
  await screenshot(page, "schedule-details.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "schedule-details-mobile.png");
  await overview.evaluate((element) =>
    element.scrollTo({ top: element.scrollHeight }),
  );
  await expect(history).toHaveScreenshot(
    "schedule-execution-history-mobile.png",
  );
  await overview
    .getByRole("button", { name: "Edit schedule", exact: true })
    .click();
  const editor = page.getByRole("dialog", {
    name: "Edit schedule",
    exact: true,
  });
  await editor
    .getByRole("textbox", { name: "Name", exact: true })
    .fill("Evening restart");
  await editor
    .getByRole("button", { name: "Save schedule", exact: true })
    .click();
  await expect(editor).toHaveCount(0);
  await page.reload();
  await page
    .getByRole("button", {
      name: "View details for Evening restart",
      exact: true,
    })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Evening restart", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Close", exact: true })
    .last()
    .click();
  await openScreen(page, "dashboard", "Overview");
  await page
    .getByRole("table", { name: "Upcoming schedules", exact: true })
    .getByRole("row")
    .filter({ hasText: "Evening restart" })
    .getByRole("cell")
    .nth(2)
    .click();
  await expect(page).toHaveURL(/#dashboard$/);
  await expect(
    page
      .getByRole("dialog", { name: "Evening restart", exact: true })
      .getByRole("button", { name: "Edit schedule", exact: true }),
  ).toBeVisible();
});

for (const permissions of [
  ["server.view"],
  ["server.view", "server.operate"],
  ["server.view", "server.configure"],
]) {
  test(`schedule overview stays read-only with ${permissions.join(", ")}`, async ({
    page,
    request,
    baseURL,
  }) => {
    await login(page, "viewer");
    const admin = await request.post("/api/v1/auth/login", {
      data: { username: "admin", password: "campfire-e2e-password" },
    });
    expect(admin.ok()).toBeTruthy();
    const { csrf_token } = (await admin.json()) as { csrf_token: string };
    const headers = { "X-CSRF-Token": csrf_token, Origin: baseURL! };
    const roleResponse = await request.post("/api/v1/roles", {
      headers,
      data: {
        name: "Schedule reader",
        scope: "server",
        permissions: [...permissions, "server.schedules.read"],
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
    await page.reload();
    await expect(
      page.getByRole("switch", {
        name: "Enable schedule Nightly restart",
        exact: true,
      }),
    ).toBeDisabled();
    await page
      .getByRole("button", {
        name: "View details for Nightly restart",
        exact: true,
      })
      .press("Enter");
    const overview = page.getByRole("dialog", {
      name: "Nightly restart",
      exact: true,
    });
    await expect(overview).toBeVisible();
    await expect(
      overview.getByRole("button", { name: "Edit schedule", exact: true }),
    ).toHaveCount(0);
    await expect(overview.getByRole("textbox")).toHaveCount(0);
    if (permissions.length === 1)
      await screenshot(page, "schedule-details-read-only.png");
    await overview.getByRole("link", { name: "Survival", exact: true }).click();
    await expect(page.getByRole("dialog")).toHaveCount(2);
    await page
      .getByRole("dialog")
      .last()
      .getByRole("button", { name: "Close", exact: true })
      .last()
      .click();
    await overview
      .getByRole("button", { name: "Close", exact: true })
      .last()
      .click();
    await openScreen(page, "schedules", "Schedules");
    await expect(
      page.getByRole("switch", {
        name: "Enable schedule Nightly restart",
        exact: true,
      }),
    ).toBeDisabled();
    await page
      .getByRole("button", {
        name: "View details for Nightly restart",
        exact: true,
      })
      .click();
    await expect(overview).toBeVisible();
    await expect(
      overview.getByRole("button", { name: "Edit schedule", exact: true }),
    ).toHaveCount(0);
    await page.goto("/#schedules/nightly-restart");
    await expect(overview).toBeVisible();
    await expect(
      overview.getByRole("button", { name: "Edit schedule", exact: true }),
    ).toHaveCount(0);
    const session = await page.request.get("/api/v1/auth/session");
    const viewer = (await session.json()) as { csrf_token: string };
    expect(
      (
        await page.request.patch("/api/v1/schedules/nightly-restart", {
          headers: { "X-CSRF-Token": viewer.csrf_token },
          data: { enabled: false },
        })
      ).status(),
    ).toBe(403);
  });
}

test("a schedule without server access cannot be opened through a direct link", async ({
  page,
}) => {
  await login(page, "viewer");
  await page.goto("/#schedules/nightly-restart");
  await expect(page).toHaveURL(/#schedules$/);
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("the seeded observer can view schedules but cannot toggle them", async ({
  page,
}) => {
  await login(page, "observer");
  const upcoming = page.getByRole("table", {
    name: "Upcoming schedules",
    exact: true,
  });
  const row = upcoming.getByRole("row").filter({ hasText: "Nightly restart" });
  await expect(
    row.getByRole("switch", {
      name: "Enable schedule Nightly restart",
      exact: true,
    }),
  ).toBeDisabled();
  await expect(upcoming).toHaveScreenshot("upcoming-schedules-read-only.png");
  await row.getByRole("cell").nth(2).click();
  const overview = page.getByRole("dialog", {
    name: "Nightly restart",
    exact: true,
  });
  await expect(
    overview.getByRole("button", { name: "Edit schedule", exact: true }),
  ).toHaveCount(0);
  await expect(
    overview
      .getByRole("region", { name: "Execution history", exact: true })
      .getByRole("row"),
  ).toHaveCount(2);
  await overview
    .getByRole("button", { name: "Close", exact: true })
    .last()
    .click();
  await openScreen(page, "schedules", "Schedules");
  await expect(
    page.getByRole("switch", {
      name: "Enable schedule Nightly restart",
      exact: true,
    }),
  ).toBeDisabled();
  await page
    .getByRole("button", {
      name: "View details for Nightly restart",
      exact: true,
    })
    .click();
  await expect(overview).toBeVisible();
  await expect(
    overview.getByRole("button", { name: "Edit schedule", exact: true }),
  ).toHaveCount(0);
  const session = await page.request.get("/api/v1/auth/session");
  const { csrf_token } = (await session.json()) as { csrf_token: string };
  expect(
    (
      await page.request.patch("/api/v1/schedules/nightly-restart", {
        headers: { "X-CSRF-Token": csrf_token },
        data: { enabled: false },
      })
    ).status(),
  ).toBe(403);
});

test("schedule history loads older attempts without including other schedules", async ({
  page,
}) => {
  await login(page);
  const at = Math.floor(new Date("2026-10-22T12:00:00Z").getTime() / 1000);
  const runs = Array.from({ length: 102 }, (_, index) => ({
    id: `attempt-${index}`,
    schedule_id: "nightly-restart",
    retry_of: "",
    recurring: true,
    server_id: "survival",
    server_name: "Survival",
    user_id: 1,
    username: "admin",
    action: "restart",
    state: "succeeded",
    message: `Attempt ${index}`,
    created_at: at - index * 60,
    started_at: at - index * 60,
    finished_at: at - index * 60 + 1,
  }));
  await page.route("**/api/v1/schedules/executions?*", (route) => {
    const query = new URL(route.request().url()).searchParams;
    if (query.get("schedule_id") !== "nightly-restart") return route.continue();
    if (query.get("before")) {
      expect(query.get("before")).toBe(`${runs[99]!.created_at}:attempt-99`);
      return route.fulfill({ json: runs.slice(100) });
    }
    return route.fulfill({ json: runs.slice(0, 100) });
  });
  await page.goto("/#schedules/nightly-restart");
  const history = page
    .getByRole("dialog")
    .getByRole("region", { name: "Execution history", exact: true });
  await expect(history.getByRole("row")).toHaveCount(101);
  await history
    .getByRole("button", { name: "Load older executions", exact: true })
    .click();
  await expect(history.getByRole("row")).toHaveCount(103);
  await expect(history.getByText("Attempt 101", { exact: true })).toBeVisible();
  await expect(
    history.getByRole("button", { name: "Load older executions", exact: true }),
  ).toHaveCount(0);
});

test("schedule history can retry a failed request and display an empty result", async ({
  page,
}) => {
  await login(page);
  let failed = true;
  await page.route("**/api/v1/schedules/executions?*", (route) => {
    if (
      new URL(route.request().url()).searchParams.get("schedule_id") !==
      "nightly-restart"
    )
      return route.continue();
    return failed
      ? route.fulfill({
          status: 408,
          json: { error: "History request timed out." },
        })
      : route.fulfill({ json: [] });
  });
  await page.goto("/#schedules/nightly-restart");
  const overview = page.getByRole("dialog", {
    name: "Nightly restart",
    exact: true,
  });
  const history = overview.getByRole("region", {
    name: "Execution history",
    exact: true,
  });
  await expect(history.getByRole("alert")).toContainText(
    "History request timed out.",
  );
  failed = false;
  await history.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(
    history.getByText("No executions recorded", { exact: true }),
  ).toBeVisible();
  await expect(
    overview.getByRole("button", { name: "Edit schedule", exact: true }),
  ).toBeEnabled();
});
