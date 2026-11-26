import { readFileSync } from "node:fs";
import { expect, test } from "vitest";

const manifest = JSON.parse(
  readFileSync("public/manifest.webmanifest", "utf8"),
) as {
  id: string;
  name: string;
  short_name: string;
  start_url: string;
  scope: string;
  display: string;
  theme_color: string;
  background_color: string;
  icons: { src: string; sizes: string; purpose: string; type: string }[];
};

test("installation has stable same-origin identity and starts at the panel, not a private deep link", () => {
  expect(manifest.id).toBe("/");
  expect(manifest.start_url).toBe("/");
  expect(manifest.scope).toBe("/");
  expect(manifest.display).toBe("standalone");
  expect(manifest.name).toBe("Campfire");
  expect(manifest.short_name).toBe("Campfire");
  expect(manifest.theme_color).toBe("#1c1c18");
  expect(manifest.background_color).toBe("#1c1c18");
});

test("manifest icons exist at their advertised PNG sizes, with a separate maskable icon", () => {
  expect(
    manifest.icons.map(({ sizes, purpose }) => `${sizes}:${purpose}`),
  ).toEqual(["192x192:any", "512x512:any", "512x512:maskable"]);
  for (const icon of manifest.icons) {
    expect(icon.src).toMatch(/^\/icons\/[a-z0-9-]+\.png$/);
    expect(icon.type).toBe("image/png");
    const png = readFileSync(`public${icon.src}`);
    expect(png.subarray(1, 4).toString()).toBe("PNG");
    expect(`${png.readUInt32BE(16)}x${png.readUInt32BE(20)}`).toBe(icon.sizes);
  }
});

test("entrypoint links the manifest and a real Apple touch icon", () => {
  const html = readFileSync("index.html", "utf8");
  const document = new DOMParser().parseFromString(html, "text/html");
  expect(
    document.querySelector('link[rel="manifest"]')?.getAttribute("href"),
  ).toBe("/manifest.webmanifest");
  expect(
    document.querySelector('meta[name="theme-color"]')?.getAttribute("content"),
  ).toBe(manifest.theme_color);
  const icon = document.querySelector('link[rel="apple-touch-icon"]');
  expect(icon?.getAttribute("href")).toBe("/icons/apple-touch-icon.png");
  const png = readFileSync("public/icons/apple-touch-icon.png");
  expect(png.readUInt32BE(16)).toBe(180);
  expect(png.readUInt32BE(20)).toBe(180);
});
