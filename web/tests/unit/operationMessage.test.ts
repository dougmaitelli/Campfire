import assert from "node:assert/strict";
import { test } from "vitest";
import { operationMessage } from "../../src/components/servers/operationMessage";

test("operation failures have context and formatted details", () => {
  assert.deepEqual(
    operationMessage({
      operation: "install",
      operation_state: "failed",
      busy: false,
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
    busy: true,
    progress: "Failed is only text",
  });
  assert.equal(result?.tone, "info");
  assert.equal(result?.title, "Server start in progress.");
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
    })?.title,
    "Server status.",
  );
});

test("interrupted operations keep their own state", () => {
  assert.equal(
    operationMessage({
      operation: "backup",
      operation_state: "interrupted",
      busy: false,
      error_code: "operation_interrupted",
    })?.title,
    "Backup interrupted.",
  );
});

test("success messages only appear for the operation started in this page session", () => {
  const status = {
    operation: "start",
    operation_id: "start-1",
    operation_state: "succeeded" as const,
    busy: false,
  };
  assert.equal(operationMessage(status), null);
  assert.equal(operationMessage(status, "start-2"), null);
  assert.deepEqual(operationMessage(status, "start-1"), {
    title: "Server start completed.",
    tone: "success",
  });
});
