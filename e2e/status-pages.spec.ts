import { test, expect, screenshot } from "./fixtures";
import { login } from "./screens";
import { readAuthenticatedSession } from "./auth";
import type { Server } from "../web/src/api/servers";
import type { Ember } from "../web/src/api/embers";
import type { StatusPage, PublicStatusPage } from "../web/src/api/statusPages";

test("admins publish selected servers on a standalone anonymous status page", async ({
  page,
  browser,
  baseURL,
}) => {
  if (!baseURL) throw new Error("Browser tests require a base URL");
  await login(page);
  const session = await readAuthenticatedSession(
    await page.request.get("/api/v1/auth/session"),
  );
  expect(
    (
      await page.request.patch("/api/v1/sparks/mock-europe", {
        headers: { "X-CSRF-Token": session.csrf_token },
        data: { game_address: "play.example.test" },
      })
    ).ok(),
  ).toBe(true);
  await page.goto("/#statusPages");
  await page
    .getByRole("button", { name: "Create status page", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Create status page",
    exact: true,
  });
  await drawer
    .getByLabel("Page title", { exact: true })
    .fill("Community availability");
  await drawer.getByLabel("URL slug", { exact: true }).fill("community-status");
  await drawer.getByRole("button", { name: "Add server", exact: true }).click();
  await drawer.getByRole("combobox", { name: "Server 1", exact: true }).click();
  await page.getByRole("option", { name: "Survival", exact: true }).click();
  await drawer
    .getByLabel("Public name for Survival", { exact: true })
    .fill("Community Minecraft");
  expect(
    await drawer
      .locator("form.panel-content")
      .evaluate((element) => getComputedStyle(element).paddingLeft),
  ).toBe("24px");
  async function expectActionsAtRowEnd() {
    expect(
      await drawer.locator(".editable-list-row-actions").evaluate((actions) => {
        const fields = actions.parentElement?.querySelector(
          ".editable-list-row-fields",
        );
        if (!fields) return false;
        return (
          actions.getBoundingClientRect().left >=
            fields.getBoundingClientRect().right &&
          new Set(
            [...actions.querySelectorAll("button")].map(
              (button) => button.getBoundingClientRect().top,
            ),
          ).size === 1
        );
      }),
    ).toBe(true);
  }
  await expectActionsAtRowEnd();
  await screenshot(page, "status-page-editor.png", drawer);
  await page.setViewportSize({ width: 390, height: 844 });
  await expectActionsAtRowEnd();
  await screenshot(page, "status-page-editor-mobile.png", drawer);
  await page.setViewportSize({ width: 1400, height: 900 });
  const created = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/status-pages") &&
      response.request().method() === "POST",
  );
  await drawer
    .getByRole("button", { name: "Create public page", exact: true })
    .click();
  const response = await created;
  expect(response.status()).toBe(201);
  const publication = (await response.json()) as StatusPage;
  await expect(drawer).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: "/status/community-status", exact: true }),
  ).toBeVisible();
  const publicationRow = page
    .getByRole("row")
    .filter({ hasText: "Community availability" });
  await expect(
    publicationRow.getByRole("button", { name: "Delete", exact: true }),
  ).toHaveCount(0);
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await publicationRow
    .getByRole("button", {
      name: "Copy URL for Community availability",
      exact: true,
    })
    .click();
  await expect(
    publicationRow.getByRole("button", {
      name: "Copied URL for Community availability",
      exact: true,
    }),
  ).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    `${baseURL}/status/community-status`,
  );
  expect(
    await publicationRow
      .locator(".status-page-copy svg")
      .evaluate((icon) => icon.getBoundingClientRect().height),
  ).toBe(
    await publicationRow
      .locator(".status-page-url a")
      .evaluate((link) => parseFloat(getComputedStyle(link).fontSize)),
  );
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const popupOpened = page.waitForEvent("popup");
  await publicationRow.getByRole("link").click();
  const linkedPage = await popupOpened;
  await linkedPage.waitForLoadState();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await linkedPage.close();
  await screenshot(page, "status-pages-populated.png");
  const anonymous = await browser.newContext({
    baseURL,
    viewport: { width: 1400, height: 900 },
    locale: "en-US",
    timezoneId: "UTC",
    reducedMotion: "reduce",
    serviceWorkers: "block",
  });
  try {
    const publicPage = await anonymous.newPage();
    // Deliberately skew the visitor clock; the feed's Campfire timestamp anchors freshness.
    await publicPage.clock.setFixedTime(new Date("2026-10-22T12:02:00Z"));
    const errors: string[] = [];
    publicPage.on("pageerror", (error) => errors.push(error.message));
    const apiRequests: string[] = [];
    publicPage.on("request", (request) => {
      if (new URL(request.url()).pathname.startsWith("/api/"))
        apiRequests.push(new URL(request.url()).pathname);
    });
    await publicPage.route(
      "**/api/v1/public/status-pages/community-status",
      async (route) => {
        const response = await route.fetch();
        if (!response.ok()) {
          await route.fulfill({ response });
          return;
        }
        const value = (await response.json()) as PublicStatusPage;
        await route.fulfill({
          response,
          json: {
            ...value,
            collected_at: 1792670400,
            entries: value.entries.map((entry) => ({
              ...entry,
              observed_at: 1792670400,
              game_state: {
                updated_at: 1792670400,
                state: {
                  players: 3,
                  capacity: 20,
                  fields: {
                    map: "Community world",
                    player_names: ["Alex", "Sam"],
                  },
                  field_labels: {
                    map: "World name",
                    player_names: "Online players",
                  },
                },
              },
            })),
          },
        });
      },
    );
    const navigation = await publicPage.goto("/status/community-status");
    expect(navigation?.headers()["x-robots-tag"]).toBe("noindex, nofollow");
    await expect(
      publicPage.getByRole("heading", {
        name: "Community availability",
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      publicPage.getByRole("heading", {
        name: "Community Minecraft",
        exact: true,
      }),
    ).toBeVisible();
    await expect(publicPage.getByRole("navigation")).toHaveCount(0);
    await expect(publicPage.getByRole("link")).toHaveCount(0);
    await expect(
      publicPage.getByRole("button", { name: /sign in|log in/i }),
    ).toHaveCount(0);
    await expect(publicPage.locator(".sidebar")).toHaveCount(0);
    await expect(publicPage.getByText("Modded", { exact: true })).toHaveCount(
      0,
    );
    await expect(publicPage.getByText("Online", { exact: true })).toBeVisible();
    await expect(
      publicPage.locator(".public-status-entry .status-pill"),
    ).toHaveCSS("border-top-width", "0px");
    await expect(
      publicPage.locator(".public-status-entry .status-pill"),
    ).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await expect(
      publicPage.getByText("play.example.test", { exact: true }),
    ).toBeVisible();
    await expect(publicPage.getByText("3 / 20", { exact: true })).toBeVisible();
    await expect(publicPage.locator(".public-status-server-icon")).toHaveCSS(
      "border-top-width",
      "0px",
    );
    await expect(publicPage.locator(".public-status-server-icon")).toHaveCSS(
      "background-color",
      "rgba(0, 0, 0, 0)",
    );
    expect(
      await publicPage
        .locator(".public-status-server-details")
        .evaluate((element) => {
          const name = element.querySelector("h3")!.getBoundingClientRect();
          const address = element
            .querySelector(".public-status-address")!
            .getBoundingClientRect();
          return address.top >= name.bottom;
        }),
    ).toBe(true);
    async function expectCheckedInCorner() {
      expect(
        await publicPage.locator(".public-status-entry").evaluate((element) => {
          const card = element.getBoundingClientRect();
          const checked = element
            .querySelector(".public-status-checked")!
            .getBoundingClientRect();
          return (
            card.right - checked.right <= 25 &&
            card.bottom - checked.bottom <= 25
          );
        }),
      ).toBe(true);
    }
    await expectCheckedInCorner();
    await expect(
      publicPage.getByText("World name", { exact: true }),
    ).toBeVisible();
    await expect(
      publicPage.getByText("Alex, Sam", { exact: true }),
    ).toBeVisible();
    await expect
      .poll(() =>
        publicPage
          .locator(".public-status-server-icon img")
          .evaluate((image: HTMLImageElement) => image.naturalWidth),
      )
      .toBeGreaterThan(0);
    expect(
      await publicPage
        .locator(".public-status-masthead")
        .evaluate((el) => el.getBoundingClientRect().height),
    ).toBeLessThanOrEqual(48);
    await expect(publicPage).toHaveScreenshot(["pages", "public-status.png"]);
    await publicPage.setViewportSize({ width: 390, height: 844 });
    await expect(publicPage).toHaveScreenshot([
      "pages",
      "public-status-mobile.png",
    ]);
    await expectCheckedInCorner();
    expect(
      await publicPage.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await anonymous.grantPermissions(["clipboard-read", "clipboard-write"]);
    await publicPage
      .getByRole("button", {
        name: "Copy game address for Community Minecraft",
        exact: true,
      })
      .click();
    await expect(
      publicPage.getByRole("button", {
        name: "Copied game address for Community Minecraft",
        exact: true,
      }),
    ).toBeVisible();
    expect(
      await publicPage.evaluate(() => navigator.clipboard.readText()),
    ).toBe("play.example.test");
    expect(errors).toEqual([]);
    expect(apiRequests.length).toBeGreaterThan(0);
    expect(
      apiRequests.every(
        (path) => path === "/api/v1/public/status-pages/community-status",
      ),
    ).toBe(true);
    const feed = await publicPage.request.get(
      "/api/v1/public/status-pages/community-status",
    );
    expect(feed.headers()["cache-control"]).toBe("no-store");
    const snapshot = (await feed.json()) as PublicStatusPage;
    expect(Object.keys(snapshot).sort()).toEqual([
      "collected_at",
      "entries",
      "poll_seconds",
      "title",
    ]);
    expect(snapshot.entries).toHaveLength(1);
    expect(Object.keys(snapshot.entries[0]!).sort()).toEqual([
      "art_url",
      "game_address",
      "id",
      "name",
      "observed_at",
      "state",
    ]);
    expect(JSON.stringify(snapshot)).not.toContain("survival");
    expect(
      (await publicPage.request.get("/api/v1/servers/survival")).status(),
    ).toBe(401);
    await publicPage.reload();
    await expect(
      publicPage.getByRole("heading", {
        name: "Community Minecraft",
        exact: true,
      }),
    ).toBeVisible();
    await publicationRow.getByRole("cell").last().click();
    const editor = page.getByRole("dialog", {
      name: "Edit status page",
      exact: true,
    });
    await editor.getByLabel("URL slug", { exact: true }).fill("renamed-status");
    await editor
      .getByRole("button", { name: "Add server", exact: true })
      .click();
    await editor
      .getByRole("combobox", { name: "Server 2", exact: true })
      .click();
    await page.getByRole("option", { name: "Modded", exact: true }).click();
    await editor
      .getByRole("button", { name: "Move Modded up", exact: true })
      .click();
    const saved = page.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/v1/status-pages/${publication.id}`) &&
        response.request().method() === "PUT",
    );
    await editor
      .getByRole("button", { name: "Save changes", exact: true })
      .click();
    expect((await saved).status()).toBe(200);
    await expect(editor).toHaveCount(0);
    await publicPage.reload();
    await expect(
      publicPage.getByRole("heading", {
        name: "Status page not found",
        exact: true,
      }),
    ).toBeVisible();
    await publicPage.goto("/status/renamed-status");
    await expect(publicPage.locator(".public-status-list h3")).toHaveText([
      "Modded",
      "Community Minecraft",
    ]);
    const signedIn = await page.context().newPage();
    try {
      const signedInRequests: string[] = [];
      signedIn.on("request", (request) => {
        const path = new URL(request.url()).pathname;
        if (path.startsWith("/api/")) signedInRequests.push(path);
      });
      await signedIn.goto("/status/renamed-status");
      await expect(signedIn.locator(".public-status-list h3")).toHaveText([
        "Modded",
        "Community Minecraft",
      ]);
      await expect(signedIn.getByRole("navigation")).toHaveCount(0);
      expect(signedInRequests).toEqual([
        "/api/v1/public/status-pages/renamed-status",
      ]);
    } finally {
      await signedIn.close();
    }
    await page
      .getByRole("button", { name: "Edit Community availability", exact: true })
      .focus();
    await page.keyboard.press("Enter");
    await page
      .getByRole("dialog", { name: "Edit status page", exact: true })
      .getByRole("button", { name: "Delete", exact: true })
      .click();
    const deleted = page.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/v1/status-pages/${publication.id}`) &&
        response.request().method() === "DELETE",
    );
    await page
      .getByRole("dialog", { name: "Delete status page", exact: true })
      .getByRole("button", { name: "Delete", exact: true })
      .click();
    expect((await deleted).status()).toBe(204);
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(
      page.getByRole("link", { name: "/status/renamed-status", exact: true }),
    ).toHaveCount(0);
    await publicPage.reload();
    await expect(
      publicPage.getByRole("heading", {
        name: "Status page not found",
        exact: true,
      }),
    ).toBeVisible();
    await expect(publicPage.getByRole("link")).toHaveCount(0);
  } finally {
    await anonymous.close();
  }
});

test("public artwork cannot normalize into a private API request for a signed-in visitor", async ({
  page,
  baseURL,
}) => {
  if (!baseURL) throw new Error("Browser tests require a base URL");
  await login(page);
  const session = await readAuthenticatedSession(
    await page.request.get("/api/v1/auth/session"),
  );
  const headers = { "X-CSRF-Token": session.csrf_token };
  // Keep the setup tab from rendering the deliberately unsafe artwork in
  // private server cards; this regression exercises the public projection.
  await page.goto("about:blank");
  const serverResponse = await page.request.get("/api/v1/servers/survival");
  expect(serverResponse.ok()).toBe(true);
  const server = (await serverResponse.json()) as Server;
  const emberResponse = await page.request.get(
    `/api/v1/embers/${server.ember_id}`,
  );
  expect(emberResponse.ok()).toBe(true);
  const ember = (await emberResponse.json()) as Ember;
  const unsafeArt =
    baseURL.replace(/^http:/, "https:") +
    String.raw`/game-art\..\api/v1/private-art`;
  expect(new URL(unsafeArt).pathname).toBe("/api/v1/private-art");
  const updated = await page.request.put(`/api/v1/embers/${ember.id}`, {
    headers,
    data: { ...ember, artUrl: unsafeArt },
  });
  expect(updated.ok(), await updated.text()).toBe(true);
  const created = await page.request.post("/api/v1/status-pages", {
    headers,
    data: {
      title: "Artwork safety",
      slug: "artwork-safety",
      entries: [{ server_id: server.id, name: "Public game" }],
    },
  });
  expect(created.status()).toBe(201);
  const feed = await page.request.get(
    "/api/v1/public/status-pages/artwork-safety",
  );
  const snapshot = (await feed.json()) as PublicStatusPage;
  expect(snapshot.entries[0]?.art_url).toBeUndefined();
  const visitor = await page.context().newPage();
  try {
    const requests: string[] = [];
    visitor.on("request", (request) => {
      const path = new URL(request.url()).pathname;
      if (path.startsWith("/api/")) requests.push(path);
    });
    await visitor.goto("/status/artwork-safety");
    await expect(
      visitor.getByRole("heading", { name: "Public game", exact: true }),
    ).toBeVisible();
    await expect(visitor.locator(".public-status-server-icon img")).toHaveCount(
      0,
    );
    await expect(
      visitor.locator(".public-status-server-icon svg"),
    ).toBeVisible();
    expect(requests).toEqual(["/api/v1/public/status-pages/artwork-safety"]);
  } finally {
    await visitor.close();
  }
});
