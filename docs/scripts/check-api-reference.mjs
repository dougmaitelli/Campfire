import { readFileSync, readdirSync } from "node:fs";

const router = new URL(
  "../../backend/internal/campfire/api/routes.go",
  import.meta.url,
);
const pages = new URL("../api/", import.meta.url);
const registered = new Set();
const documented = new Set();

for (const match of readFileSync(router, "utf8").matchAll(
  /"([A-Z]+) \/api\/v1(\/[^"]*)"/g,
)) {
  registered.add(`${match[1]} ${match[2]}`);
}

for (const filename of readdirSync(pages)) {
  if (!filename.endsWith(".md")) continue;
  const page = readFileSync(new URL(filename, pages), "utf8");
  for (const match of page.matchAll(/\|[ \t]*`([A-Z]+ \/[^`\s]+)`[ \t]*\|/g)) {
    documented.add(match[1]);
  }
}

for (const route of registered.difference(documented)) {
  console.error(`Missing from the panel API reference: ${route}`);
  process.exitCode = 1;
}
for (const route of documented.difference(registered)) {
  console.error(`No matching panel API route: ${route}`);
  process.exitCode = 1;
}
