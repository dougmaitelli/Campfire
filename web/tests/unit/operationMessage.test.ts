import {
  ServerAction,
  OperationState,
  RuntimeErrorCode,
} from "../../src/contracts/domains";
import assert from "node:assert/strict";
import { test } from "vitest";
import { operationMessage } from "../../src/components/servers/operationMessage";

test("operation failures have context and formatted details", () => {
  assert.deepEqual(
    operationMessage({
      operation: ServerAction.Install,
      operation_state: OperationState.Failed,
      busy: false,
      error_code: RuntimeErrorCode.InstallerFailed,
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
    operation: ServerAction.Start,
    operation_state: OperationState.Running,
    busy: true,
    progress: "Failed is only text",
  });
  assert.equal(result?.tone, "info");
  assert.equal(result?.title, "Server start in progress.");
});

test("an error remains visible when no operation state or code is supplied", () => {
  assert.deepEqual(
    operationMessage({
      operation: ServerAction.Install,
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
      operation: ServerAction.Install,
      busy: false,
      progress: "Any text",
    })?.title,
    "Server status.",
  );
});

test("interrupted operations keep their own state", () => {
  assert.equal(
    operationMessage({
      operation: ServerAction.Backup,
      operation_state: OperationState.Interrupted,
      busy: false,
      error_code: RuntimeErrorCode.OperationInterrupted,
    })?.title,
    "Backup interrupted.",
  );
});

test("success messages only appear for the operation started in this page session", () => {
  const status = {
    operation: ServerAction.Start,
    operation_id: "start-1",
    operation_state: OperationState.Succeeded,
    busy: false,
  };
  assert.equal(operationMessage(status), null);
  assert.equal(operationMessage(status, "start-2"), null);
  assert.equal(operationMessage({ ...status, busy: true }, "start-1"), null);
  assert.deepEqual(operationMessage(status, "start-1"), {
    title: "Server start completed.",
    tone: "success",
  });
});
