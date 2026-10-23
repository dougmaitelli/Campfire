import { defineConfig } from "@playwright/test";

process.env.NO_PROXY = [process.env.NO_PROXY, "127.0.0.1", "localhost"]
  .filter(Boolean)
  .join(",");

export default defineConfig({
  testDir: ".",
  testMatch: "**/*.spec.ts",
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 30_000,
  updateSnapshots: "none",
  expect: {
    timeout: 10_000,
    toHaveScreenshot: {
      animations: "disabled",
      caret: "hide",
      maxDiffPixels: 0,
    },
  },
  snapshotPathTemplate: "{testDir}/../screenshots/{arg}{ext}",
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: "http://127.0.0.1:8099",
    browserName: "chromium",
    viewport: { width: 1400, height: 900 },
    locale: "en-US",
    timezoneId: "UTC",
    reducedMotion: "reduce",
    serviceWorkers: "block",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [{ name: "desktop", use: { colorScheme: "dark" } }],
  webServer: {
    command: "node scripts/start.mjs",
    wait: { stdout: /Campfire E2E listening on 127\.0\.0\.1:8099/ },
    timeout: 600_000,
    gracefulShutdown: { signal: "SIGTERM", timeout: 10_000 },
  },
});
