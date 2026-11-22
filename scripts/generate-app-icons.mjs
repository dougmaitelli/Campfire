/* global Image, document */ // Browser globals used inside page.evaluate.
// Run after installing e2e dependencies and Google Chrome:
// node scripts/generate-app-icons.mjs
// Icons are checked in; normal builds do not require a browser.
import { readFile, mkdir, writeFile } from "node:fs/promises";
import { chromium } from "../e2e/node_modules/@playwright/test/index.mjs";

const directory = new URL("../web/public/icons/", import.meta.url);
const svg = await readFile(
  new URL("../web/public/brand/campfire.svg", import.meta.url),
  "utf8",
);
const browser = await chromium.launch({ channel: "chrome", headless: true });
try {
  const page = await browser.newPage();
  await mkdir(directory, { recursive: true });
  for (const [name, size, scale] of [
    ["app-192.png", 192, 0.8],
    ["app-512.png", 512, 0.8],
    ["app-maskable-512.png", 512, 0.56],
    ["apple-touch-icon.png", 180, 0.8],
  ]) {
    const data = await page.evaluate(
      async ({ svg, size, scale }) => {
        const image = new Image();
        image.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
        await image.decode();
        const canvas = document.createElement("canvas");
        canvas.width = canvas.height = size;
        const context = canvas.getContext("2d");
        context.fillStyle = "#1c1c18";
        context.fillRect(0, 0, size, size);
        // The maskable artwork fits entirely inside the central 80% safe circle.
        const height = size * scale;
        const width = (height * 64) / 68;
        context.drawImage(
          image,
          (size - width) / 2,
          (size - height) / 2,
          width,
          height,
        );
        return canvas.toDataURL("image/png").split(",")[1];
      },
      { svg, size, scale },
    );
    await writeFile(new URL(name, directory), Buffer.from(data, "base64"));
  }
} finally {
  await browser.close();
}
