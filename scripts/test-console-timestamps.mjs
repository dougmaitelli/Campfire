import assert from "node:assert/strict";
import test from "node:test";
import { loadWebModule } from "./helpers/load-web-module.mjs";

const { ConsoleTimestamps } = await loadWebModule(
  new URL(
    "../web/src/components/servers/consoleTimestamps.ts",
    import.meta.url,
  ),
);

test("formats Docker timestamps across arbitrary chunk boundaries", () => {
  const source =
    "2026-10-21T18:14:12.123456789Z \x1b[32mReady\x1b[0m\n2026-10-21T18:14:13Z > ";
  const localTime = (iso) => {
    const date = new Date(iso);
    return [date.getHours(), date.getMinutes(), date.getSeconds()]
      .map((part) => String(part).padStart(2, "0"))
      .join(":");
  };
  const expected = `[${localTime("2026-10-21T18:14:12Z")}] \x1b[32mReady\x1b[0m\n[${localTime("2026-10-21T18:14:13Z")}] > `;
  for (let split = 0; split <= source.length; split++) {
    const formatter = new ConsoleTimestamps();
    assert.equal(
      formatter.format(source.slice(0, split)) +
        formatter.format(source.slice(split)) +
        formatter.flush(),
      expected,
    );
  }
});

test("preserves process timestamps, ANSI colors, prompts and unfinished prefixes", () => {
  const formatter = new ConsoleTimestamps();
  const content = "[INFO] Ready\n\x1b[31mError\x1b[0m\n> ";
  assert.equal(formatter.format(content), content);
  assert.equal(formatter.format("\n2026-10"), "\n");
  assert.equal(formatter.flush(), "2026-10");
  formatter.reset();
  assert.equal(formatter.format("Prompt>"), "Prompt>");
});
