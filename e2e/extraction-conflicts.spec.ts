import {
  ExtractionState,
  WorkPhase,
  ExtractionOutcomeStatus,
  FileOperation,
  ExtractionAction,
} from "../web/src/contracts/domains";
import type { Page } from "@playwright/test";
import { test, expect } from "./fixtures";
import { login, openServerTab } from "./screens";
import type {
  ExtractionConflict,
  ExtractionStatus,
  FileRequest,
} from "../web/src/api/fileTypes";
import { FileKind } from "../web/src/contracts/domains";

const metadata = {
  name: "level.dat",
  kind: FileKind.File,
  size: 24,
  modified_at: "2026-11-01T00:00:00Z",
  mode: 0o600,
};
const member: ExtractionConflict = {
  id: "level-conflict",
  path: "level.dat",
  destination: "world/level.dat",
  kind: FileKind.File,
  size: 12,
  existing: metadata,
};
const conflict = (page: Page) =>
  page.getByRole("dialog", {
    name: "Resolve extraction conflict",
    exact: true,
  });
async function choose(page: Page, action: string) {
  await conflict(page)
    .getByRole("radio", { name: action, exact: true })
    .check();
  await conflict(page)
    .getByRole("button", { name: "Continue", exact: true })
    .click();
}

async function begin(
  page: Page,
  options: {
    conflicts?: ExtractionConflict[];
    failure?: "stale" | "lost" | "duplicate" | "expired" | "lost-cancel";
    destination?: "." | "./";
    running?: boolean;
    measured?: boolean;
    expireDuringWait?: boolean;
    completed?: boolean;
  } = {},
) {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page.locator('input[type="file"]').setInputFiles({
    name: "world.zip",
    mimeType: "application/zip",
    buffer: Buffer.from("Native Spark tests cover archive decoding."),
  });
  await expect(
    page.getByText("1 file uploaded.", { exact: true }),
  ).toBeVisible();
  const calls: FileRequest[] = [];
  const queue = [...(options.conflicts ?? [member])];
  let first = true;
  let status: ExtractionStatus = {
    id: "",
    version: 1,
    state: options.running
      ? ExtractionState.Running
      : queue.length
        ? ExtractionState.Waiting
        : ExtractionState.Completed,
    expires_at: options.expireDuringWait
      ? "2026-10-22T12:00:00.100Z"
      : "2026-11-18T12:00:00Z",
    pending_conflicts: options.running ? 0 : queue.length,
    remaining_entries: queue.length,
    outcomes: [
      ...(options.completed !== false
        ? [
            {
              path: "new.txt",
              destination: "world/new.txt",
              status: ExtractionOutcomeStatus.Extracted,
            },
          ]
        : []),
      ...queue.map((item) => ({
        path: item.path,
        destination: item.destination,
        status: ExtractionOutcomeStatus.Pending,
      })),
    ],
    ...(!options.running && queue[0] ? { conflict: queue[0] } : {}),
  };
  if (options.measured) {
    status.work_progress = {
      phase: WorkPhase.Extracting,
      input_bytes: 1024,
      input_total: 4096,
      output_bytes: 8192,
      entries: 2,
      updated_at: new Date(await page.evaluate(() => Date.now())).toISOString(),
    };
  }
  if (options.failure === "duplicate" || options.failure === "expired") {
    status = {
      ...status,
      state:
        options.failure === "expired"
          ? ExtractionState.Expired
          : ExtractionState.Failed,
      message:
        options.failure === "expired"
          ? "Extraction expired while waiting for a response."
          : 'Archive contains duplicate path "save.dat". Earlier entries were kept.',
      pending_conflicts: 0,
      remaining_entries: 0,
      outcomes: status.outcomes.map((item) => ({
        ...item,
        status:
          item.status === ExtractionOutcomeStatus.Pending
            ? ExtractionOutcomeStatus.Cancelled
            : item.status,
      })),
    };
    delete status.conflict;
  }
  await page.route("**/api/v1/servers/survival/files", async (route) => {
    const input = route.request().postDataJSON() as FileRequest;
    if (!input.operation.startsWith("extract.")) return route.continue();
    calls.push(input);
    expect(input.path).toBe("/world.zip");
    expect(input.destination).toBe(options.destination ? "/" : "/world");
    status.id = input.extraction!.id;
    if (
      input.operation === FileOperation.ExtractCancel &&
      options.failure === "lost-cancel"
    )
      return route.abort("connectionfailed");
    if (
      input.operation === FileOperation.ExtractStatus &&
      options.expireDuringWait
    ) {
      status = {
        ...status,
        version: status.version + 1,
        state: ExtractionState.Expired,
        pending_conflicts: 0,
        remaining_entries: 0,
        message: "Extraction expired while waiting for a response.",
        outcomes: status.outcomes.map((item) => ({
          ...item,
          status:
            item.status === ExtractionOutcomeStatus.Pending
              ? ExtractionOutcomeStatus.Cancelled
              : item.status,
        })),
      };
      delete status.conflict;
    }
    if (
      input.operation === FileOperation.ExtractResolve &&
      status.conflict?.id === input.extraction?.conflict_id
    ) {
      if (options.failure === "stale" && first) {
        first = false;
        queue[0] = { ...queue[0]!, id: "changed-conflict", changed: true };
        status = {
          ...status,
          version: status.version + 1,
          conflict: queue[0]!,
          message: "The destination changed. Review this conflict again.",
        };
      } else {
        const chosen = queue.shift()!;
        const action = input.extraction?.action;
        status.outcomes = status.outcomes.map((item) =>
          item.path === chosen.path
            ? {
                ...item,
                status:
                  action === ExtractionAction.Skip
                    ? ExtractionOutcomeStatus.Skipped
                    : action === ExtractionAction.Rename
                      ? ExtractionOutcomeStatus.Renamed
                      : ExtractionOutcomeStatus.Overwritten,
              }
            : chosen.kind === FileKind.Directory &&
                item.path.startsWith(chosen.path + "/")
              ? {
                  ...item,
                  status:
                    action === ExtractionAction.Skip
                      ? ExtractionOutcomeStatus.Skipped
                      : ExtractionOutcomeStatus.Extracted,
                }
              : item,
        );
        status = {
          ...status,
          version: status.version + 1,
          state: queue.length
            ? ExtractionState.Waiting
            : ExtractionState.Completed,
          pending_conflicts: queue.length,
          remaining_entries: queue.length,
        };
        delete status.message;
        if (queue[0]) status.conflict = queue[0];
        else delete status.conflict;
        if (options.failure === "lost" && first) {
          first = false;
          return route.abort("connectionfailed");
        }
      }
    }
    if (
      input.operation === FileOperation.ExtractCancel &&
      [ExtractionState.Running, ExtractionState.Waiting].includes(status.state)
    ) {
      status = {
        ...status,
        version: status.version + 1,
        state: ExtractionState.Cancelled,
        pending_conflicts: 0,
        remaining_entries: 0,
        outcomes: status.outcomes.map((item) => ({
          ...item,
          status:
            item.status === ExtractionOutcomeStatus.Pending
              ? ExtractionOutcomeStatus.Cancelled
              : item.status,
        })),
      };
      delete status.conflict;
    }
    return route.fulfill({
      json: {
        server_id: "survival",
        path: options.destination ? "/" : "/world/",
        extraction: status,
      },
    });
  });
  await page
    .getByRole("button", { name: "Extract world.zip", exact: true })
    .click();
  const input = page.getByRole("dialog", {
    name: "Extract archive",
    exact: true,
  });
  if (options.destination)
    await input
      .getByRole("textbox", { name: "Destination folder" })
      .fill(options.destination);
  await input.getByRole("button", { name: "Extract", exact: true }).click();
  return calls;
}

test("extraction asks about an actual file, not the destination, on desktop and mobile", async ({
  page,
}) => {
  const calls = await begin(page);
  await expect(
    conflict(page).getByText(/Choose what to do with level.dat/),
  ).toBeVisible();
  await expect(
    page.getByRole("dialog", {
      name: "Extract into existing folder",
      exact: true,
    }),
  ).toHaveCount(0);
  await expect(conflict(page).getByRole("checkbox")).toHaveCount(0);
  await page.evaluate(() => document.fonts.ready);
  await page.mouse.move(0, 0);
  await expect(conflict(page)).toHaveScreenshot([
    "crops",
    "file-extract-conflict.png",
  ]);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(conflict(page)).toHaveScreenshot([
    "crops",
    "file-extract-conflict-mobile.png",
  ]);
  await choose(page, "Overwrite");
  await expect(
    page.getByText(
      "Extraction results: 2 completed, 0 skipped, 0 failed, 0 not extracted.",
      { exact: true },
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("dialog", { name: "Review extraction" }),
  ).toHaveCount(0);
  expect(
    calls.find((call) => call.operation === FileOperation.ExtractResolve)
      ?.extraction,
  ).toMatchObject({ conflict_id: "level-conflict", action: "overwrite" });
});
test("an open conflict closes on confirmed expiry without another decision", async ({
  page,
}) => {
  const calls = await begin(page, { expireDuringWait: true });
  await expect(
    page.getByText(
      "Extraction results: 1 completed, 0 skipped, 1 failed, 1 not extracted.",
      { exact: true },
    ),
  ).toBeVisible();
  await expect(conflict(page)).toHaveCount(0);
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractStatus),
  ).toBe(true);
  expect(
    calls.some(
      (call) =>
        call.operation === FileOperation.ExtractResolve ||
        call.operation === FileOperation.ExtractCancel,
    ),
  ).toBe(false);
});

test("known conflicts support apply-to-all while completed entries stay completed", async ({
  page,
}) => {
  const calls = await begin(page, {
    conflicts: [
      member,
      {
        ...member,
        id: "other",
        path: "other.dat",
        destination: "world/other.dat",
      },
    ],
  });
  await conflict(page)
    .getByRole("checkbox", { name: "Apply this action to remaining conflicts" })
    .check();
  await choose(page, "Skip");
  await expect(
    page.getByText(
      "Extraction results: 1 completed, 2 skipped, 0 failed, 0 not extracted.",
      { exact: true },
    ),
  ).toBeVisible();
  expect(
    calls
      .filter((call) => call.operation === FileOperation.ExtractResolve)
      .map((call) => call.extraction?.action),
  ).toEqual(["skip", "skip"]);
});
test("a directory blocking a file permits rename, not overwrite", async ({
  page,
}) => {
  await begin(page, {
    conflicts: [
      { ...member, existing: { ...metadata, kind: FileKind.Directory } },
    ],
  });
  await expect(
    conflict(page).getByRole("radio", { name: "Overwrite", exact: true }),
  ).toBeDisabled();
  await conflict(page)
    .getByRole("radio", { name: "Rename", exact: true })
    .check();
  await conflict(page)
    .getByRole("textbox", { name: "New filename" })
    .fill("level copy.dat");
  await conflict(page)
    .getByRole("button", { name: "Continue", exact: true })
    .click();
  await expect(
    page.getByText(
      "Extraction results: 2 completed, 0 skipped, 0 failed, 0 not extracted.",
      { exact: true },
    ),
  ).toBeVisible();
});
test("changed destinations require a fresh answer instead of applying stale overwrite", async ({
  page,
}) => {
  const calls = await begin(page, { failure: "stale" });
  await choose(page, "Overwrite");
  await expect(
    conflict(page).getByText(
      "The destination changed. Review this conflict again.",
      { exact: true },
    ),
  ).toBeVisible();
  expect(
    calls.filter((call) => call.operation === FileOperation.ExtractResolve),
  ).toHaveLength(1);
  await choose(page, "Skip");
  await expect(
    page.getByText(
      "Extraction results: 1 completed, 1 skipped, 0 failed, 0 not extracted.",
      { exact: true },
    ),
  ).toBeVisible();
});
test("a lost decision reply is recovered without restarting extraction", async ({
  page,
}) => {
  const calls = await begin(page, { failure: "lost" });
  await choose(page, "Overwrite");
  await expect(
    page.getByText(
      "Extraction results: 2 completed, 0 skipped, 0 failed, 0 not extracted.",
      { exact: true },
    ),
  ).toBeVisible();
  expect(
    calls.filter((call) => call.operation === FileOperation.ExtractStart),
  ).toHaveLength(1);
  const answers = calls.filter(
    (call) => call.operation === FileOperation.ExtractResolve,
  );
  expect(answers).toHaveLength(2);
  expect(answers[0]?.extraction).toEqual(answers[1]?.extraction);
});
for (const failure of ["duplicate", "expired"] as const) {
  test(`${failure} extraction reports partial results rather than rollback or success`, async ({
    page,
  }) => {
    await begin(page, { failure });
    await expect(
      page.getByText(
        "Extraction results: 1 completed, 0 skipped, 1 failed, 1 not extracted.",
        { exact: true },
      ),
    ).toBeVisible();
    await expect(
      page.getByText(
        "Some entries could not be extracted. Check the entry results; completed entries are kept.",
        { exact: true },
      ),
    ).toBeVisible();
  });
}
for (const destination of [".", "./"] as const) {
  test(`destination ${destination} needs no root conflict or upfront review`, async ({
    page,
  }) => {
    const calls = await begin(page, { destination, conflicts: [] });
    await expect(
      page.getByText(
        "Extraction results: 1 completed, 0 skipped, 0 failed, 0 not extracted.",
        { exact: true },
      ),
    ).toBeVisible();
    await expect(conflict(page)).toHaveCount(0);
    expect(
      calls.find((call) => call.operation === FileOperation.ExtractStart)
        ?.destination,
    ).toBe("/");
  });
}
test("cancelling a conflict keeps completed entries and cancels unresolved entries", async ({
  page,
}) => {
  const calls = await begin(page);
  await conflict(page)
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await expect(
    page.getByText(
      "Extraction results: 1 completed, 0 skipped, 0 failed, 1 not extracted.",
      { exact: true },
    ),
  ).toBeVisible();
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractCancel),
  ).toBe(true);
});
for (const completed of [false, true]) {
  test(`cancelling a scanner does not report finished with ${completed ? "some" : "no"} completed entries`, async ({
    page,
  }) => {
    const calls = await begin(page, {
      running: true,
      conflicts: [],
      completed,
    });
    const dialog = page.getByRole("dialog", {
      name: "Extract archive",
      exact: true,
    });
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(
      page.getByText(
        `Extraction results: ${completed ? 1 : 0} completed, 0 skipped, 0 failed, 0 not extracted.`,
        { exact: true },
      ),
    ).toBeVisible();
    await expect(
      page.getByText("Extraction finished. Check the entry results below.", {
        exact: true,
      }),
    ).toHaveCount(0);
    expect(
      calls.some((call) => call.operation === FileOperation.ExtractCancel),
    ).toBe(true);
  });
  test(`lost scanner cancellation keeps ${completed ? "some" : "no"} confirmed entries and marks completion unknown`, async ({
    page,
  }) => {
    await begin(page, {
      running: true,
      conflicts: [],
      completed,
      failure: "lost-cancel",
    });
    const dialog = page.getByRole("dialog", {
      name: "Extract archive",
      exact: true,
    });
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(
      page.getByText(
        "Extraction completion could not be confirmed. Check the destination before retrying.",
        { exact: true },
      ),
    ).toBeVisible();
    await expect(
      page.getByText(
        `Extraction results: ${completed ? 1 : 0} completed, 0 skipped, 0 failed, 0 not extracted, 1 unconfirmed.`,
        { exact: true },
      ),
    ).toBeVisible();
    await expect(
      page.getByText("Extraction finished. Check the entry results below.", {
        exact: true,
      }),
    ).toHaveCount(0);
  });
}

test("measured extraction progress appears inside the active input dialog", async ({
  page,
}) => {
  await begin(page, { running: true, measured: true });
  const dialog = page.getByRole("dialog", {
    name: "Extract archive",
    exact: true,
  });
  await expect(
    dialog.getByRole("group", {
      name: "Extracting and validating archive",
      exact: true,
    }),
  ).toBeVisible();
  await expect(dialog.getByRole("progressbar")).toHaveAttribute(
    "value",
    "1024",
  );
  await expect(page.locator(".work-progress")).toHaveCount(1);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.locator(".work-progress")).toHaveCount(0);
});

test("extraction can be cancelled while it is processing rather than waiting on a conflict", async ({
  page,
}) => {
  const calls = await begin(page, { running: true });
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(
    page.getByText(
      "Extraction results: 1 completed, 0 skipped, 0 failed, 1 not extracted.",
      { exact: true },
    ),
  ).toBeVisible();
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractCancel),
  ).toBe(true);
});
