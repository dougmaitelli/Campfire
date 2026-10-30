import { test, expect } from "./fixtures";
import { login } from "./screens";

test("game activity loads protected screenshots and playable clips", async ({
  page,
}) => {
  await login(page);
  await page.goto("/#servers/survival/activity");
  const image = page.getByRole("img", { name: "mock-world.png" });
  await expect(image).toBeVisible();
  await expect
    .poll(() =>
      image.evaluate((element) => (element as HTMLImageElement).naturalWidth),
    )
    .toBe(320);
  const video = page.getByLabel("mock-clip.webm", { exact: true });
  await expect(video).toBeVisible();
  await video.evaluate(async (element) => {
    const clip = element as HTMLVideoElement;
    clip.load();
    await clip.play();
    clip.pause();
  });
  expect(
    await video.evaluate((element) => (element as HTMLVideoElement).duration),
  ).toBeGreaterThan(0);
  const source = await video.locator("source").getAttribute("src");
  if (!source) throw new Error("Missing clip URL");
  const range = await page.request.get(source, {
    headers: { Range: "bytes=0-15" },
  });
  expect(range.status()).toBe(206);
  expect((await range.body()).length).toBe(16);
  const download = await page.request.get(`${source}?download=1`);
  expect(download.headers()["content-disposition"]).toContain("attachment");
  expect(download.headers()["cache-control"]).toContain("no-store");
});

test("game media cannot be downloaded without server access", async ({
  page,
}) => {
  await login(page);
  const response = await page.request.get("/api/v1/servers/survival/events");
  const events = (await response.json()) as {
    items: { id: number; attachments?: { id: string }[] }[];
  };
  const event = events.items.find((item) => item.attachments?.length);
  if (!event) throw new Error("Missing media fixture");
  const url = `/api/v1/servers/survival/events/${event.id}/media/screenshot`;
  const anonymous = await page.request.get(url, { headers: { Cookie: "" } });
  expect(anonymous.status()).toBe(401);
  await page.context().clearCookies();
  await login(page, "viewer");
  expect((await page.request.get(url)).status()).toBe(404);
});
