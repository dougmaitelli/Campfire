import { beforeAll, afterAll, beforeEach, expect, test, vi } from "vitest";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ServerFileBrowser } from "../../src/components/servers/ServerFileBrowser";
import { APIError } from "../../src/api/client";
import {
  manageFiles,
  uploadLimit,
  type FileEntry,
  type FileRequest,
  type ExtractionStatus,
  type ExtractionConflict,
} from "../../src/api/files";
import type { Server } from "../../src/api/servers";
import { FileKind, FileOperation } from "../../src/contracts/domains";
import { availableFileName } from "../../src/components/files/fileNames";
import { FileConflictDialog } from "../../src/components/files/FileConflictDialog";
import { useFileConflicts } from "../../src/components/files/useFileConflicts";
import type { ResolveFileConflict } from "../../src/components/files/fileConflicts";

vi.mock("../../src/api/files", async () => ({
  ...(await vi.importActual("../../src/api/files")),
  manageFiles: vi.fn(),
  uploadLimit: vi.fn(),
}));

const descriptors = new Map<string, PropertyDescriptor | undefined>();
beforeAll(() => {
  for (const name of ["showModal", "close"]) {
    descriptors.set(
      name,
      Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype, name),
    );
    Object.defineProperty(HTMLDialogElement.prototype, name, {
      configurable: true,
      value: function (this: HTMLDialogElement) {
        if (name === "showModal") this.setAttribute("open", "");
        else this.removeAttribute("open");
      },
    });
  }
});
afterAll(() => {
  for (const [name, descriptor] of descriptors) {
    if (descriptor)
      Object.defineProperty(HTMLDialogElement.prototype, name, descriptor);
    else Reflect.deleteProperty(HTMLDialogElement.prototype, name);
  }
});

const server: Server = {
  id: "game",
  name: "Game",
  description: "",
  spark_id: "spark",
  ember_id: "ember",
  image: "game:latest",
  variables: {},
  cpu_limit: 1,
  memory_mb: 1024,
  disk_mb: 1024,
  ports: [],
  ember_hash: "hash",
  ember_update_available: false,
  owner_user_id: 1,
  permissions: [
    "server.view",
    "server.console.read",
    "server.operate",
    "server.files.read",
    "server.files.write",
  ],
  spark_name: "Spark",
  ember_name: "Game",
  game_address: "",
  status: "running",
  created_at: "2026-11-01",
};
const entry = (
  name: string,
  kind: FileEntry["kind"] = FileKind.File,
): FileEntry => ({
  name,
  kind,
  size: 3,
  modified_at: "2026-11-01T00:00:00Z",
  mode: 0o600,
});
let files: Map<string, { entry: FileEntry; revision: string }>;
let calls: FileRequest[];
let stages: Map<string, FileRequest>;
let serial: number;
let commitError: Error | null;
let statError: APIError | null;
let extractionState: ExtractionStatus;
let extractionConflicts: ExtractionConflict[];
let extractionAnswer: ((input: FileRequest) => boolean) | null;
let extractionAfterAnswer: (() => void) | null;
let extractionOnStatus: (() => void) | null;
beforeEach(() => {
  files = new Map();
  calls = [];
  stages = new Map();
  serial = 0;
  commitError = null;
  statError = null;
  configureExtraction([]);
  extractionAnswer = null;
  extractionAfterAnswer = null;
  extractionOnStatus = null;
  vi.mocked(uploadLimit).mockResolvedValue(1024 ** 3);
  vi.mocked(manageFiles)
    .mockReset()
    .mockImplementation(async (id, input) => {
      calls.push(input);
      const result = { server_id: id, path: input.path };
      const name = input.path.slice(1);
      if (input.operation === FileOperation.List) {
        const prefix =
          input.path === "/" ? "" : `${input.path.replace(/^\/|\/$/g, "")}/`;
        return {
          ...result,
          entries: [...files.entries()]
            .filter(
              ([key]) =>
                key.startsWith(prefix) &&
                !key.slice(prefix.length).includes("/"),
            )
            .map(([key, item]) => ({
              ...item.entry,
              name: key.slice(prefix.length),
            })),
        };
      }
      if (input.operation.startsWith("extract.")) {
        extractionState.id = input.extraction!.id;
        if (input.operation === FileOperation.ExtractStatus)
          extractionOnStatus?.();
        if (input.operation === FileOperation.ExtractResolve) {
          if (
            !extractionAnswer?.(input) &&
            extractionState.conflict?.id === input.extraction?.conflict_id
          ) {
            const chosen = extractionConflicts.shift()!;
            const action = input.extraction?.action;
            extractionState.outcomes = extractionState.outcomes.map((item) =>
              item.path === chosen.path
                ? {
                    ...item,
                    status:
                      action === "skip"
                        ? "skipped"
                        : action === "rename"
                          ? "renamed"
                          : action === "overwrite"
                            ? "overwritten"
                            : "extracted",
                  }
                : item,
            );
            extractionState.version++;
            extractionState.pending_conflicts = extractionConflicts.length;
            extractionState.remaining_entries = extractionConflicts.length;
            extractionState.state = extractionConflicts.length
              ? "waiting"
              : "completed";
            delete extractionState.message;
            if (extractionConflicts[0])
              extractionState.conflict = extractionConflicts[0];
            else delete extractionState.conflict;
            extractionAfterAnswer?.();
          }
        }
        if (input.operation === FileOperation.ExtractCancel) {
          extractionState = {
            ...extractionState,
            version: extractionState.version + 1,
            state: "cancelled",
            pending_conflicts: 0,
            remaining_entries: 0,
            outcomes: extractionState.outcomes.map((item) => ({
              ...item,
              status: item.status === "pending" ? "cancelled" : item.status,
            })),
          };
          delete extractionState.conflict;
        }
        return { ...result, extraction: structuredClone(extractionState) };
      }
      if (input.operation === FileOperation.Stat) {
        if (statError) throw statError;
        if (input.path === "/")
          return {
            ...result,
            revision: "root-revision",
            entries: [entry("/", FileKind.Directory)],
          };
        const found = files.get(name);
        if (!found) throw new APIError("File not found.", 404);
        return { ...result, revision: found.revision, entries: [found.entry] };
      }
      if (input.operation === FileOperation.UploadBegin) {
        if (input.revision !== (files.get(name)?.revision ?? "absent"))
          throw new APIError("Destination changed or already exists.", 409);
        const upload_id = `upload-${++serial}`;
        stages.set(upload_id, input);
        return { ...result, upload_id };
      }
      if (input.operation === FileOperation.UploadCommit) {
        if (commitError) throw commitError;
        const stage = stages.get(input.upload_id!);
        if (stage?.revision !== (files.get(name)?.revision ?? "absent"))
          throw new APIError("Destination changed during upload.", 409);
        files.set(name, { entry: entry(name), revision: `revision-${serial}` });
        stages.delete(input.upload_id!);
      }
      if (input.operation === FileOperation.UploadCancel)
        stages.delete(input.upload_id!);
      return result;
    });
});
function existing(name: string, kind: FileEntry["kind"] = FileKind.File) {
  files.set(name, { entry: entry(name, kind), revision: `original-${name}` });
}
async function start(names: string[]) {
  const user = userEvent.setup();
  const view = render(<ServerFileBrowser server={server} canWrite />);
  await waitFor(() =>
    expect(
      screen
        .getAllByRole("button", { name: "Upload Files" })[0]!
        .hasAttribute("disabled"),
    ).toBe(false),
  );
  const input =
    view.container.querySelector<HTMLInputElement>('input[type="file"]')!;
  await user.upload(
    input,
    names.map((name) => new File([], name)),
  );
  return { user, view };
}
async function choose(
  user: ReturnType<typeof userEvent.setup>,
  action: string,
  all = false,
  operation: "upload" | "extract" = "upload",
) {
  const dialog = await screen.findByRole("dialog", {
    name:
      operation === "upload"
        ? "Resolve upload conflict"
        : /^(Resolve extraction conflict|Extract into existing folder)$/,
  });
  await user.click(within(dialog).getByRole("radio", { name: action }));
  if (all)
    await user.click(
      within(dialog).getByRole("checkbox", {
        name: "Apply this action to remaining conflicts",
      }),
    );
  await user.click(within(dialog).getByRole("button", { name: "Continue" }));
}

test("uploads new files without prompting and reports every completed file", async () => {
  await start(["one.txt", "two.txt"]);
  await screen.findByText("2 files uploaded.");
  expect(
    screen.getByText(
      "Upload results: 2 completed, 0 skipped, 0 failed, 0 not uploaded.",
    ),
  ).toBeDefined();
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(files.has("two.txt")).toBe(true);
});
test("skip-all conflicts preserves existing files and still uploads nonconflicting files", async () => {
  existing("one.txt");
  existing("two.txt");
  const { user } = await start(["one.txt", "two.txt", "new.txt"]);
  expect(
    calls.filter((c) => c.operation === FileOperation.UploadBegin),
  ).toHaveLength(0);
  await choose(user, "Skip", true);
  await screen.findByText(
    "Upload results: 1 completed, 2 skipped, 0 failed, 0 not uploaded.",
  );
  expect(files.get("one.txt")?.revision).toBe("original-one.txt");
  expect(
    calls
      .filter((c) => c.operation === FileOperation.UploadBegin)
      .map((c) => c.path),
  ).toEqual(["/new.txt"]);
});
test("overwrite uses the observed revision, never an unconditional overwrite flag", async () => {
  existing("one.txt");
  const { user } = await start(["one.txt"]);
  await choose(user, "Overwrite");
  await screen.findByText("1 file uploaded.");
  expect(
    calls.find((c) => c.operation === FileOperation.UploadBegin)?.revision,
  ).toBe("original-one.txt");
  expect(files.get("one.txt")?.revision).not.toBe("original-one.txt");
  expect(screen.getByText(/Existing file replaced/)).toBeDefined();
});
test("rename-all generates distinct names and does not replace existing files", async () => {
  existing("one.txt");
  existing("one copy.txt");
  existing("two.txt");
  const { user } = await start(["one.txt", "two.txt"]);
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("radio", { name: "Rename" }));
  expect(
    within(dialog).getByRole<HTMLInputElement>("textbox", {
      name: "New filename",
    }).value,
  ).toBe("one copy 2.txt");
  await user.click(within(dialog).getByRole("checkbox"));
  await user.click(within(dialog).getByRole("button", { name: "Continue" }));
  await screen.findByText("2 files uploaded.");
  expect(files.has("one copy 2.txt")).toBe(true);
  expect(files.has("two copy.txt")).toBe(true);
  expect(files.get("one.txt")?.revision).toBe("original-one.txt");
});
test("rename validates occupied names and path traversal before continuing", async () => {
  existing("one.txt");
  const { user } = await start(["one.txt", "future.txt"]);
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("radio", { name: "Rename" }));
  const input = within(dialog).getByRole("textbox", { name: "New filename" });
  for (const value of ["../escape", "future.txt", "one.txt"]) {
    await user.clear(input);
    await user.type(input, value);
    expect(
      within(dialog)
        .getByRole("button", { name: "Continue" })
        .hasAttribute("disabled"),
    ).toBe(true);
  }
  await user.clear(input);
  await user.type(input, "chosen.txt");
  await user.click(within(dialog).getByRole("button", { name: "Continue" }));
  await screen.findByText("2 files uploaded.");
  expect(files.has("chosen.txt")).toBe(true);
});
test("duplicate names within a batch prompt rather than silently dropping a file", async () => {
  const { user } = await start(["one.txt", "one.txt"]);
  await screen.findByText("Another file in this upload batch uses this name.");
  await choose(user, "Skip");
  await screen.findByText(
    "Upload results: 1 completed, 1 skipped, 0 failed, 0 not uploaded.",
  );
  expect(
    calls.filter((c) => c.operation === FileOperation.UploadCommit),
  ).toHaveLength(1);
});
test("directories cannot be overwritten, even after applying overwrite-all", async () => {
  existing("one.txt");
  existing("folder", FileKind.Directory);
  const { user } = await start(["one.txt", "folder"]);
  await choose(user, "Overwrite", true);
  await screen.findByText(
    "The destination is not a regular file and cannot be overwritten.",
  );
  const dialog = await screen.findByRole("dialog");
  expect(
    within(dialog)
      .getByRole("radio", { name: "Overwrite" })
      .hasAttribute("disabled"),
  ).toBe(true);
  await choose(user, "Skip");
  await screen.findByText(
    "Upload results: 1 completed, 1 skipped, 0 failed, 0 not uploaded.",
  );
});
test("closing a conflict cancels remaining files and reports completed work", async () => {
  existing("conflict.txt");
  const { user } = await start(["new.txt", "conflict.txt", "remaining.txt"]);
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await screen.findByText(
    "Upload results: 1 completed, 0 skipped, 0 failed, 2 not uploaded.",
  );
  expect(
    screen.queryByText("Upload cancelled. Completed files are kept."),
  ).toBeNull();
  expect(
    screen.getByText(
      "Upload results: 1 completed, 0 skipped, 0 failed, 2 not uploaded.",
    ),
  ).toBeDefined();
  expect(files.has("remaining.txt")).toBe(false);
});
test("failed metadata lookup never falls back to uploading", async () => {
  statError = new APIError("Permission denied.", 403);
  await start(["one.txt", "two.txt"]);
  await screen.findByText(
    "Upload results: 0 completed, 0 skipped, 2 failed, 0 not uploaded.",
  );
  expect(calls.some((c) => c.operation === FileOperation.UploadBegin)).toBe(
    false,
  );
});
test("a concurrent change at commit reports failure and cleans the staging session", async () => {
  existing("one.txt");
  commitError = new APIError("Destination changed during upload.", 409);
  const { user } = await start(["one.txt"]);
  await choose(user, "Overwrite");
  await screen.findByText(
    "Some files could not be uploaded. Check the upload results.",
  );
  expect(
    screen.getByText(
      "Upload results: 0 completed, 0 skipped, 1 failed, 0 not uploaded.",
    ),
  ).toBeDefined();
  expect(calls.some((c) => c.operation === FileOperation.UploadCancel)).toBe(
    true,
  );
  expect(files.get("one.txt")?.revision).toBe("original-one.txt");
  expect(screen.queryByText("1 file uploaded.")).toBeNull();
});
test("unmounting while choosing a conflict never starts the upload", async () => {
  existing("one.txt");
  const { view } = await start(["one.txt"]);
  await screen.findByRole("dialog");
  view.unmount();
  await waitFor(() =>
    expect(calls.some((c) => c.operation === FileOperation.UploadBegin)).toBe(
      false,
    ),
  );
});
test("Escape cancels a conflict without uploading", async () => {
  existing("one.txt");
  await start(["one.txt"]);
  fireEvent(
    await screen.findByRole("dialog"),
    new Event("cancel", { cancelable: true }),
  );
  await screen.findByText(
    "Upload results: 0 completed, 0 skipped, 0 failed, 1 not uploaded.",
  );
});
test("a changed destination while the dialog is open fails instead of overwriting", async () => {
  existing("one.txt");
  const { user } = await start(["one.txt"]);
  await screen.findByRole("dialog");
  files.set("one.txt", { entry: entry("one.txt"), revision: "concurrent" });
  await choose(user, "Overwrite");
  await screen.findByText(
    "Upload results: 0 completed, 0 skipped, 1 failed, 0 not uploaded.",
  );
  expect(files.get("one.txt")?.revision).toBe("concurrent");
  expect(calls.some((c) => c.operation === FileOperation.UploadCommit)).toBe(
    false,
  );
});
test("a lost commit response is unconfirmed, not falsely failed or cancelled", async () => {
  commitError = new DOMException("Response lost", "AbortError");
  await start(["one.txt", "remaining.txt"]);
  await screen.findByText(
    "Upload completion could not be confirmed. Check the destination before retrying.",
  );
  expect(
    screen.getByText(
      "Upload results: 0 completed, 0 skipped, 0 failed, 1 not uploaded, 1 unconfirmed.",
    ),
  ).toBeDefined();
  expect(
    calls.filter((c) => c.operation === FileOperation.UploadBegin),
  ).toHaveLength(1);
});
test("overwrite-all makes explicit revision-checked replacements for every conflict", async () => {
  existing("one.txt");
  existing("two.txt");
  const { user } = await start(["one.txt", "two.txt"]);
  await choose(user, "Overwrite", true);
  await screen.findByText("2 files uploaded.");
  expect(
    calls
      .filter((c) => c.operation === FileOperation.UploadBegin)
      .map((c) => c.revision),
  ).toEqual(["original-one.txt", "original-two.txt"]);
});
test("large batches still check and report every duplicate rather than bypassing preflight", async () => {
  existing("one.txt");
  const { user } = await start(Array.from({ length: 1001 }, () => "one.txt"));
  await choose(user, "Skip", true);
  await screen.findByText(
    "Upload results: 0 completed, 1001 skipped, 0 failed, 0 not uploaded.",
  );
  expect(calls.filter((c) => c.operation === FileOperation.Stat)).toHaveLength(
    1001,
  );
  expect(calls.some((c) => c.operation === FileOperation.UploadBegin)).toBe(
    false,
  );
});
test("generated rename suggestions preserve extensions and dotfiles", () => {
  expect(
    availableFileName("world.tar.gz", new Set(["world copy.tar.gz"])),
  ).toBe("world copy 2.tar.gz");
  expect(availableFileName(".env", new Set())).toBe(".env copy");
});

test("a single upload conflict hides apply-to-all even when nonconflicting files follow", async () => {
  existing("one.txt");
  const { user } = await start(["one.txt", "fresh.txt"]);
  const dialog = await screen.findByRole("dialog", {
    name: "Resolve upload conflict",
  });
  expect(within(dialog).queryByRole("checkbox")).toBeNull();
  await choose(user, "Skip");
  await screen.findByText(
    "Upload results: 1 completed, 1 skipped, 0 failed, 0 not uploaded.",
  );
});

test("apply-to-all disappears on the last upload conflict", async () => {
  existing("one.txt");
  existing("two.txt");
  const { user } = await start(["one.txt", "two.txt"]);
  let dialog = await screen.findByRole("dialog", {
    name: "Resolve upload conflict",
  });
  expect(within(dialog).getByRole("checkbox")).toBeDefined();
  await choose(user, "Skip");
  dialog = await screen.findByRole("dialog", {
    name: "Resolve upload conflict",
  });
  expect(within(dialog).queryByRole("checkbox")).toBeNull();
  await choose(user, "Skip");
  await screen.findByText(
    "Upload results: 0 completed, 2 skipped, 0 failed, 0 not uploaded.",
  );
});

test("rename rejects the original name even without a reservation and cannot confirm it", async () => {
  const onResolve = vi.fn();
  const user = userEvent.setup();
  render(
    <FileConflictDialog
      operation="upload"
      hasRemainingConflicts={false}
      conflict={{
        name: "one.txt",
        size: 3,
        existing: entry("one.txt"),
        duplicate: false,
        suggestedName: "one copy.txt",
      }}
      reserved={new Set()}
      onResolve={onResolve}
    />,
  );
  const dialog = screen.getByRole("dialog");
  await user.click(within(dialog).getByRole("radio", { name: "Rename" }));
  const input = within(dialog).getByRole("textbox", { name: "New filename" });
  await user.clear(input);
  await user.type(input, "one.txt");
  expect(
    within(dialog).getByRole<HTMLButtonElement>("button", { name: "Continue" })
      .disabled,
  ).toBe(true);
  await user.click(within(dialog).getByRole("button", { name: "Continue" }));
  expect(onResolve).not.toHaveBeenCalled();
  await user.clear(input);
  await user.type(input, "different.txt");
  await user.click(within(dialog).getByRole("button", { name: "Continue" }));
  expect(onResolve).toHaveBeenCalledWith({
    action: "rename",
    name: "different.txt",
    applyToAll: false,
  });
});

async function startExtraction(destination?: string) {
  existing("bundle.zip");
  existing("bundle", FileKind.Directory);
  const view = await start([]);
  await view.user.click(
    screen.getByRole("button", { name: "Extract bundle.zip" }),
  );
  const dialog = screen.getByRole("dialog", { name: "Extract archive" });
  if (destination) {
    const input = within(dialog).getByRole("textbox", {
      name: "Destination folder",
    });
    await view.user.clear(input);
    await view.user.type(input, destination);
  }
  await view.user.click(
    within(dialog).getByRole("button", { name: "Extract" }),
  );
  return view;
}
function configureExtraction(names: string[] = ["one.txt"], completed = true) {
  extractionConflicts = names.map((name, index) => ({
    id: `conflict-${index}`,
    path: name,
    destination: `bundle/${name}`,
    kind: FileKind.File,
    size: 3,
    existing: entry(name),
  }));
  extractionState = {
    id: "",
    version: 1,
    state: names.length ? "waiting" : "completed",
    expires_at: new Date(Date.now() + 900000).toISOString(),
    pending_conflicts: names.length,
    remaining_entries: names.length,
    outcomes: [
      ...(completed
        ? [
            {
              path: "fresh.txt",
              destination: "bundle/fresh.txt",
              status: "extracted" as const,
            },
          ]
        : []),
      ...names.map((name) => ({
        path: name,
        destination: `bundle/${name}`,
        status: "pending" as const,
      })),
    ],
    ...(extractionConflicts[0] ? { conflict: extractionConflicts[0] } : {}),
  };
}

test("existing destination directories are not conflicts; unrelated entries are already extracted", async () => {
  configureExtraction();
  existing("bundle/one.txt");
  const { user } = await startExtraction();
  const dialog = await screen.findByRole("dialog", {
    name: "Resolve extraction conflict",
  });
  expect(
    within(dialog).getByText(/Choose what to do with one.txt/),
  ).toBeDefined();
  expect(
    screen.queryByRole("dialog", { name: "Extract into existing folder" }),
  ).toBeNull();
  expect(
    screen.queryByRole("dialog", { name: "Review extraction" }),
  ).toBeNull();
  expect(extractionState.outcomes[0]?.status).toBe("extracted");
  expect(within(dialog).queryByRole("checkbox")).toBeNull();
  await choose(user, "Overwrite", false, "extract");
  await screen.findByText(
    "Extraction results: 2 completed, 0 skipped, 0 failed, 0 not extracted.",
  );
  const decision = calls.find(
    (call) => call.operation === FileOperation.ExtractResolve,
  );
  expect(decision?.extraction).toMatchObject({
    conflict_id: "conflict-0",
    action: "overwrite",
  });
});
test("aborting a dispatched fresh-folder extraction reports unconfirmed completion", async () => {
  const implementation = vi.mocked(manageFiles).getMockImplementation()!;
  let dispatched = false;
  vi.mocked(manageFiles).mockImplementation(async (...args) => {
    if (args[1].operation === FileOperation.Extract) {
      dispatched = true;
      return new Promise((_, reject) => {
        args[2]!.addEventListener(
          "abort",
          () => reject(new DOMException("Response aborted", "AbortError")),
          { once: true },
        );
      });
    }
    return implementation(...args);
  });
  const { user } = await startExtraction("fresh folder");
  expect(dispatched).toBe(true);
  const dialog = screen.getByRole("dialog", { name: "Extract archive" });
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await screen.findByText(
    "Extraction completion could not be confirmed. Check the destination before retrying.",
  );
  expect(
    screen.queryByText("Extraction finished. Check the entry results below."),
  ).toBeNull();
  expect(
    screen.getByText(
      "Extraction results: 0 completed, 0 skipped, 0 failed, 0 not extracted, 1 unconfirmed.",
    ),
  ).toBeDefined();
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractCancel),
  ).toBe(false);
});

test("a fresh folder uses direct one-shot extraction without creating a conflict operation", async () => {
  configureExtraction();
  await startExtraction("fresh folder");
  await screen.findByText(
    "Extraction results: 1 completed, 0 skipped, 0 failed, 0 not extracted.",
  );
  expect(
    calls.find((call) => call.operation === FileOperation.Extract)?.destination,
  ).toBe("/fresh folder");
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractStart),
  ).toBe(false);
});

test("extraction without collisions completes without a destination question or final review", async () => {
  configureExtraction([]);
  await startExtraction();
  await screen.findByText(
    "Extraction results: 1 completed, 0 skipped, 0 failed, 0 not extracted.",
  );
  expect(
    calls.filter((call) => call.operation === FileOperation.ExtractStart),
  ).toHaveLength(1);
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractResolve),
  ).toBe(false);
});
test("apply-to-all handles known conflicts without affecting already extracted entries", async () => {
  configureExtraction(["one.txt", "two.txt"]);
  const { user } = await startExtraction();
  await choose(user, "Skip", true, "extract");
  await screen.findByText(
    "Extraction results: 1 completed, 2 skipped, 0 failed, 0 not extracted.",
  );
  expect(
    calls
      .filter((call) => call.operation === FileOperation.ExtractResolve)
      .map((call) => call.extraction?.action),
  ).toEqual(["skip", "skip"]);
});
test("the last extraction conflict hides apply-to-all", async () => {
  configureExtraction(["one.txt", "two.txt"]);
  const { user } = await startExtraction();
  await choose(user, "Skip", false, "extract");
  const dialog = await screen.findByRole("dialog", {
    name: "Resolve extraction conflict",
  });
  expect(within(dialog).queryByRole("checkbox")).toBeNull();
  await choose(user, "Skip", false, "extract");
  await screen.findByText(
    "Extraction results: 1 completed, 2 skipped, 0 failed, 0 not extracted.",
  );
});
test("extraction refuses file-over-directory overwrite and allows a new name", async () => {
  configureExtraction();
  extractionConflicts[0]!.existing = entry("one.txt", FileKind.Directory);
  const { user } = await startExtraction();
  const dialog = await screen.findByRole("dialog", {
    name: "Resolve extraction conflict",
  });
  expect(
    within(dialog).getByRole<HTMLInputElement>("radio", { name: "Overwrite" })
      .disabled,
  ).toBe(true);
  await user.click(within(dialog).getByRole("radio", { name: "Rename" }));
  await user.click(within(dialog).getByRole("button", { name: "Continue" }));
  await screen.findByText(
    "Extraction results: 2 completed, 0 skipped, 0 failed, 0 not extracted.",
  );
  expect(
    calls.find((call) => call.operation === FileOperation.ExtractResolve)
      ?.extraction?.action,
  ).toBe("rename");
});
test("a stale overwrite requires another explicit answer even after apply-to-all", async () => {
  configureExtraction(["one.txt", "two.txt"]);
  let first = true;
  extractionAnswer = () => {
    if (first) {
      first = false;
      extractionConflicts[0] = {
        ...extractionConflicts[0]!,
        id: "changed-conflict",
        changed: true,
      };
      extractionState = {
        ...extractionState,
        version: extractionState.version + 1,
        conflict: extractionConflicts[0]!,
        message: "The destination changed. Review this conflict again.",
      };
      return true;
    }
    return false;
  };
  const { user } = await startExtraction();
  await choose(user, "Overwrite", true, "extract");
  const dialog = await screen.findByRole("dialog", {
    name: "Resolve extraction conflict",
  });
  expect(
    within(dialog).getByText(
      "The destination changed. Review this conflict again.",
    ),
  ).toBeDefined();
  expect(
    calls.filter((call) => call.operation === FileOperation.ExtractResolve),
  ).toHaveLength(1);
  await choose(user, "Skip", false, "extract");
  await screen.findByText(
    "Extraction results: 2 completed, 1 skipped, 0 failed, 0 not extracted.",
  );
});
test("cancelling a conflict keeps completed entries and cancels only unresolved work", async () => {
  configureExtraction(["one.txt", "two.txt"]);
  const { user } = await startExtraction();
  const dialog = await screen.findByRole("dialog", {
    name: "Resolve extraction conflict",
  });
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await screen.findByText(
    "Extraction results: 1 completed, 0 skipped, 0 failed, 2 not extracted.",
  );
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractCancel),
  ).toBe(true);
});
test("unmounting while waiting sends cancellation instead of a conflict decision", async () => {
  configureExtraction();
  const { view } = await startExtraction();
  await screen.findByRole("dialog", { name: "Resolve extraction conflict" });
  view.unmount();
  await waitFor(() =>
    expect(
      calls.some((call) => call.operation === FileOperation.ExtractCancel),
    ).toBe(true),
  );
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractResolve),
  ).toBe(false);
});
test("a lost decision response is retried with the exact idempotent token", async () => {
  configureExtraction();
  let lost = true;
  extractionAfterAnswer = () => {
    if (lost) {
      lost = false;
      throw new TypeError("Failed to fetch");
    }
  };
  const { user } = await startExtraction();
  await choose(user, "Overwrite", false, "extract");
  await screen.findByText(
    "Extraction results: 2 completed, 0 skipped, 0 failed, 0 not extracted.",
  );
  const decisions = calls.filter(
    (call) => call.operation === FileOperation.ExtractResolve,
  );
  expect(decisions).toHaveLength(2);
  expect(decisions[0]?.extraction).toEqual(decisions[1]?.extraction);
});
test("late archive failure retains partial results and reports the failure", async () => {
  configureExtraction([]);
  extractionState.state = "failed";
  extractionState.message =
    "Invalid archive checksum. Earlier entries were kept.";
  await startExtraction();
  await screen.findByText(
    "Extraction results: 1 completed, 0 skipped, 1 failed, 0 not extracted.",
  );
});
test("expired conflict waits report stopped work rather than successful extraction", async () => {
  configureExtraction();
  extractionState.state = "expired";
  extractionState.pending_conflicts = 0;
  extractionState.remaining_entries = 0;
  delete extractionState.conflict;
  extractionState.outcomes = extractionState.outcomes.map((item) => ({
    ...item,
    status: item.status === "pending" ? "cancelled" : item.status,
  }));
  extractionState.message = "Extraction expired while waiting for a response.";
  await startExtraction();
  await screen.findByText(
    "Extraction results: 1 completed, 0 skipped, 1 failed, 1 not extracted.",
  );
});
test.each([false, true])(
  "confirmed scanner cancellation preserves encountered results (completed=%s) without a finished banner",
  async (completed) => {
    configureExtraction([], completed);
    extractionState.state = "running";
    const { user } = await startExtraction();
    const dialog = screen.getByRole("dialog", { name: "Extract archive" });
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(
        calls.some((call) => call.operation === FileOperation.ExtractCancel),
      ).toBe(true),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Extract archive" }),
      ).toBeNull(),
    );
    expect(
      screen.queryByText("Extraction finished. Check the entry results below."),
    ).toBeNull();
    expect(
      screen.queryByText(
        "Extraction completion could not be confirmed. Check the destination before retrying.",
      ),
    ).toBeNull();
    expect(
      screen.getByText(
        `Extraction results: ${completed ? 1 : 0} completed, 0 skipped, 0 failed, 0 not extracted.`,
      ),
    ).toBeDefined();
  },
);

test.each([false, true])(
  "lost cancellation of an active scanner marks the operation unconfirmed (completed=%s)",
  async (completed) => {
    configureExtraction([], completed);
    extractionState.state = "running";
    const implementation = vi.mocked(manageFiles).getMockImplementation()!;
    vi.mocked(manageFiles).mockImplementation(async (...args) => {
      if (args[1].operation === FileOperation.ExtractCancel) {
        calls.push(args[1]);
        throw new TypeError("Failed to fetch");
      }
      return implementation(...args);
    });
    const { user } = await startExtraction();
    const dialog = screen.getByRole("dialog", { name: "Extract archive" });
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await screen.findByText(
      "Extraction completion could not be confirmed. Check the destination before retrying.",
    );
    expect(
      screen.queryByText("Extraction finished. Check the entry results below."),
    ).toBeNull();
    expect(
      screen.getByText(
        `Extraction results: ${completed ? 1 : 0} completed, 0 skipped, 0 failed, 0 not extracted, 1 unconfirmed.`,
      ),
    ).toBeDefined();
  },
);

test("an open conflict dismisses itself on expiry without requiring a decision", async () => {
  configureExtraction();
  extractionState.expires_at = new Date(Date.now() + 100).toISOString();
  extractionOnStatus = () => {
    extractionState.version++;
    extractionState.state = "expired";
    extractionState.pending_conflicts = 0;
    extractionState.remaining_entries = 0;
    delete extractionState.conflict;
    extractionState.outcomes = extractionState.outcomes.map((item) => ({
      ...item,
      status: item.status === "pending" ? "cancelled" : item.status,
    }));
    extractionState.message =
      "Extraction expired while waiting for a response.";
  };
  await startExtraction();
  await screen.findByText(
    "Extraction results: 1 completed, 0 skipped, 1 failed, 1 not extracted.",
  );
  expect(
    screen.queryByRole("dialog", { name: "Resolve extraction conflict" }),
  ).toBeNull();
  expect(
    calls.some(
      (call) =>
        call.operation === FileOperation.ExtractResolve ||
        call.operation === FileOperation.ExtractCancel,
    ),
  ).toBe(false);
});

test("a failed conflict observation closes the prompt and preserves results during cancellation", async () => {
  configureExtraction();
  extractionState.expires_at = new Date(Date.now() - 100).toISOString();
  extractionOnStatus = () => {
    throw new APIError("Could not observe extraction.", 403);
  };
  await startExtraction();
  await screen.findByText(
    "Extraction results: 1 completed, 0 skipped, 1 failed, 1 not extracted.",
  );
  expect(
    screen.queryByRole("dialog", { name: "Resolve extraction conflict" }),
  ).toBeNull();
  expect(screen.getByText(/Could not observe extraction\./)).toBeDefined();
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractResolve),
  ).toBe(false);
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractCancel),
  ).toBe(true);
});

test("an early browser deadline keeps the prompt open until the server confirms expiry", async () => {
  configureExtraction();
  extractionState.expires_at = new Date(Date.now() - 100).toISOString();
  let observations = 0;
  extractionOnStatus = () => {
    observations++;
    if (observations < 2) return;
    extractionState.version++;
    extractionState.state = "expired";
    extractionState.pending_conflicts = 0;
    extractionState.remaining_entries = 0;
    delete extractionState.conflict;
    extractionState.outcomes = extractionState.outcomes.map((item) => ({
      ...item,
      status: item.status === "pending" ? "cancelled" : item.status,
    }));
  };
  await startExtraction();
  expect(
    await screen.findByRole("dialog", { name: "Resolve extraction conflict" }),
  ).toBeDefined();
  await screen.findByText(
    "Extraction results: 1 completed, 0 skipped, 1 failed, 1 not extracted.",
    {},
    { timeout: 3000 },
  );
  expect(observations).toBeGreaterThanOrEqual(2);
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractCancel),
  ).toBe(false);
});

test("aborting a conflict promise closes the dialog and releases its resolver", async () => {
  let resolve!: ResolveFileConflict;
  function Harness() {
    const conflicts = useFileConflicts();
    resolve = conflicts.createSession("extract");
    return conflicts.dialog;
  }
  render(<Harness />);
  const controller = new AbortController();
  const conflict = {
    name: "one.txt",
    size: 1,
    existing: entry("one.txt"),
    duplicate: false,
  };
  let pending!: ReturnType<ResolveFileConflict>;
  act(() => {
    pending = resolve(conflict, {
      reserved: new Set(),
      signal: controller.signal,
    });
  });
  await screen.findByRole("dialog", { name: "Resolve extraction conflict" });
  await act(async () => {
    controller.abort();
    expect(await pending).toBeNull();
  });
  expect(screen.queryByRole("dialog")).toBeNull();
  await expect(
    resolve(conflict, { reserved: new Set(), signal: controller.signal }),
  ).resolves.toBeNull();
  act(() => {
    pending = resolve(conflict, { reserved: new Set() });
  });
  await screen.findByRole("dialog", { name: "Resolve extraction conflict" });
  fireEvent(
    screen.getByRole("dialog"),
    new Event("cancel", { cancelable: true }),
  );
  await expect(pending).resolves.toBeNull();
});

test.each([".", "./"])(
  "destination %s extracts into the current folder without a root conflict",
  async (destination) => {
    configureExtraction([]);
    await startExtraction(destination);
    await screen.findByText(
      "Extraction results: 1 completed, 0 skipped, 0 failed, 0 not extracted.",
    );
    expect(
      calls.find((call) => call.operation === FileOperation.ExtractStart)
        ?.destination,
    ).toBe("/");
    expect(screen.queryByText(/Choose what to do with \//)).toBeNull();
  },
);
test("metadata failures do not fall back to blind extraction", async () => {
  configureExtraction();
  statError = new APIError("Permission denied.", 403);
  await startExtraction();
  await screen.findByText("Permission denied.");
  expect(
    calls.some((call) => call.operation === FileOperation.ExtractStart),
  ).toBe(false);
});
