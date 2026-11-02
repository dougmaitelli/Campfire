import { expect, test } from "vitest";
import { completeEmber } from "../fixtures/ember";
import {
  createEmberDraft,
  emberDraftIssue,
  serializeEmberDraft,
} from "../../src/components/embers/emberDraft";

test("visual and JSON round trips retain all advanced definition fields", () => {
  const definition = completeEmber();
  expect(serializeEmberDraft(createEmberDraft(definition))).toEqual(definition);
});

test("editing a basic field preserves nested options without mutating the imported definition", () => {
  const original = completeEmber();
  const draft = createEmberDraft(original);
  draft.definition.name = "Edited game";
  draft.definition.variables![0]!.validation!.min = 5;
  draft.images[0]!.name = "Latest Alpine";
  const edited = serializeEmberDraft(draft);
  expect(original.name).toBe("Authoring fixture");
  expect(original.variables![0]!.validation!.min).toBe(1);
  expect(edited.name).toBe("Edited game");
  expect(edited.variables![0]!.validation!.min).toBe(5);
  expect(edited.images).toEqual({
    "Latest Alpine": "alpine:3.22",
    Debian: "debian:bookworm-slim",
  });
  expect(edited.configs).toEqual(original.configs);
  expect(edited.readiness).toEqual(original.readiness);
  expect(edited.origin).toEqual(original.origin);
});

test.each(["images", "commands", "replacements"])(
  "unfinished %s rows cannot overwrite other entries",
  (group) => {
    const draft = createEmberDraft(completeEmber());
    const rows =
      group === "images"
        ? draft.images
        : group === "commands"
          ? draft.commands
          : draft.files[0]!.values;
    rows.push({ name: rows[0]!.name, value: "another value" });
    expect(emberDraftIssue(draft)).toContain("unique");
    expect(() => serializeEmberDraft(draft)).toThrow();
    rows[rows.length - 1]!.name = "";
    expect(emberDraftIssue(draft)).toContain("unnamed");
    expect(() => serializeEmberDraft(draft)).toThrow();
    rows[rows.length - 1]!.name = "Another";
    expect(emberDraftIssue(draft)).toBeNull();
    expect(() => serializeEmberDraft(draft)).not.toThrow();
  },
);

test("the command count determines single or named startup syntax", () => {
  const draft = createEmberDraft(completeEmber());
  draft.commands = [{ name: "Default", value: "sleep 60" }];
  expect(serializeEmberDraft(draft).startup_commands).toBeUndefined();
  expect(serializeEmberDraft(draft).startup).toBe("sleep 60");
  draft.commands.push({ name: "Debug", value: "sleep 120" });
  expect(serializeEmberDraft(draft).startup_commands).toEqual({
    Default: "sleep 60",
    Debug: "sleep 120",
  });
  expect(serializeEmberDraft(draft).startup).toBe("");
});

test("an unfinished config row does not prevent fixing fields in another step", () => {
  const draft = createEmberDraft(completeEmber());
  draft.files[0]!.values.push({ name: "", value: "" });
  expect(emberDraftIssue(draft, "files")).not.toBeNull();
  expect(emberDraftIssue(draft, "runtime")).toBeNull();
});

test("empty configuration maps and optional variable lists can be edited", () => {
  const definition = completeEmber();
  definition.variables = null;
  definition.configs![0]!.values = null;
  const draft = createEmberDraft(definition);
  expect(draft.files[0]!.values).toEqual([]);
  expect(serializeEmberDraft(draft).variables).toBeNull();
});

test("readiness conditions choose the matching schema without changing their meaning", () => {
  const draft = createEmberDraft(completeEmber());
  draft.readiness = {
    any: [{ value: "Ready", regex: false }],
    strip_ansi: false,
  };
  expect(serializeEmberDraft(draft).ready).toBe("Ready");
  expect(serializeEmberDraft(draft).readiness).toBeUndefined();
  draft.readiness.any[0]!.regex = true;
  expect(serializeEmberDraft(draft).ready).toBeUndefined();
  expect(serializeEmberDraft(draft).readiness).toEqual(draft.readiness);
  draft.readiness.any[0]!.regex = false;
  draft.readiness.strip_ansi = true;
  expect(serializeEmberDraft(draft).readiness).toEqual(draft.readiness);
  draft.waitForReadyMessage = false;
  expect(serializeEmberDraft(draft).ready).toBeUndefined();
  expect(serializeEmberDraft(draft).readiness).toBeUndefined();
  draft.waitForReadyMessage = true;
  expect(serializeEmberDraft(draft).readiness).toEqual(draft.readiness);
  draft.readiness.any = [];
  expect(() => serializeEmberDraft(draft)).toThrow(/readiness condition/);
});
