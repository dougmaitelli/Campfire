import { expect, test } from "vitest";
import { FileKind } from "../../src/contracts/domains";
import {
  conflictActions,
  type FileConflict,
} from "../../src/components/files/fileConflicts";

const file: FileConflict = {
  name: "save.dat",
  kind: FileKind.File,
  size: 4,
  duplicate: false,
  suggestedName: "save copy.dat",
  existing: {
    name: "save.dat",
    kind: FileKind.File,
    size: 3,
    modified_at: "2026-11-01T00:00:00Z",
  },
};

test("regular file conflicts allow explicit overwrite", () => {
  expect(conflictActions(file)).toEqual(["skip", "overwrite", "rename"]);
});
test("protected sources never offer overwrite", () => {
  expect(conflictActions({ ...file, protected: true })).toEqual([
    "skip",
    "rename",
  ]);
});
test("file and directory collisions cannot overwrite", () => {
  expect(conflictActions({ ...file, kind: FileKind.Directory })).toEqual([
    "skip",
    "rename",
  ]);
});
test("compatible directories offer merge", () => {
  expect(
    conflictActions({
      ...file,
      kind: FileKind.Directory,
      existing: { ...file.existing!, kind: FileKind.Directory },
    }),
  ).toEqual(["skip", "merge", "rename"]);
});
test("an observed removal offers extract, but an unresolved batch duplicate does not", () => {
  expect(conflictActions({ ...file, existing: null, changed: true })).toEqual([
    "skip",
    "extract",
    "rename",
  ]);
  expect(
    conflictActions({
      ...file,
      existing: null,
      changed: true,
      duplicate: true,
    }),
  ).toEqual(["skip", "rename"]);
});
