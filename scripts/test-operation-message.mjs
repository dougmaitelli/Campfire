import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { stripTypeScriptTypes } from "node:module";
import test from "node:test";

const sourceURL = new URL(
  "../web/src/components/servers/operationMessage.ts",
  import.meta.url,
);
const errorsURL = new URL("../web/src/utils/errors.ts", import.meta.url);
const source = stripTypeScriptTypes(await readFile(sourceURL, "utf8")).replace(
  '"../../utils/errors"',
  JSON.stringify(errorsURL.href),
);
const { operationMessage } = await import(
  `data:text/javascript;base64,${Buffer.from(source).toString("base64")}`
);

test("operation failures have context and formatted details", () => {
  assert.deepEqual(
    operationMessage({
      operation: "install",
      operation_state: "failed",
      error_code: "installer_failed",
      progress: "Any display text",
      error: "installer script failed",
    }),
    {
      title: "Installation failed.",
      detail: "Installer script failed.",
      tone: "error",
    },
  );
});

test("display text does not decide whether an operation failed", () => {
  const result = operationMessage({
    operation: "start",
    operation_state: "running",
    progress: "Failed is only text",
  });
  assert.equal(result.tone, "info");
  assert.equal(result.title, "Server start in progress.");
});

test("interrupted operations keep their own state", () => {
  assert.equal(
    operationMessage({
      operation: "backup",
      operation_state: "interrupted",
      error_code: "operation_interrupted",
    }).title,
    "Backup interrupted.",
  );
});
