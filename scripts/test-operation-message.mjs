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

test("an error remains visible when no operation state or code is supplied", () => {
  assert.deepEqual(
    operationMessage({
      operation: "install",
      busy: false,
      progress: "Failed",
      error:
        "docker operation failed; inspect Docker and server logs on the Spark",
    }),
    {
      title: "Installation failed.",
      detail:
        "Docker operation failed; inspect Docker and server logs on the Spark.",
      tone: "error",
    },
  );
});

test("an idle operation is not described as in progress", () => {
  assert.equal(
    operationMessage({
      operation: "install",
      busy: false,
      progress: "Any text",
    }).title,
    "Server status.",
  );
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

test("success messages only appear for the operation started in this page session", () => {
  const status = {
    operation: "start",
    operation_id: "start-1",
    operation_state: "succeeded",
    busy: false,
  };
  assert.equal(operationMessage(status), null);
  assert.equal(operationMessage(status, "start-2"), null);
  assert.deepEqual(operationMessage(status, "start-1"), {
    title: "Server start completed.",
    tone: "success",
  });
});
