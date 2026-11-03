import type { Ember, EmberConfigFile } from "../../api/embers";
import i18n from "../../i18n";

export interface NamedValue {
  name: string;
  value: string;
}

export interface EmberFileDraft {
  file: EmberConfigFile;
  values: NamedValue[];
}

export interface EmberDraft {
  definition: Ember;
  images: NamedValue[];
  commands: NamedValue[];
  files: EmberFileDraft[];
  waitForReadyMessage: boolean;
  readiness: NonNullable<Ember["readiness"]>;
}

function entries(values: Record<string, string>): NamedValue[] {
  return Object.entries(values).map(([name, value]) => ({ name, value }));
}

export function emptyEmber(): Ember {
  return {
    id: "",
    schema: 1,
    name: "",
    description: "",
    images: {},
    startup: "",
    startup_shell: "sh",
    work_dir: "/home/container",
    user: "1000:1000",
    variables: [],
  };
}

export function createEmberDraft(value: Ember): EmberDraft {
  const definition = structuredClone(value);
  return {
    definition,
    images: entries(definition.images),
    commands: Object.keys(definition.startup_commands ?? {}).length
      ? entries(definition.startup_commands!)
      : [{ name: "Default", value: definition.startup }],
    files: (definition.configs ?? []).map((file) => ({
      file,
      values: entries(file.values ?? {}),
    })),
    waitForReadyMessage: !!definition.ready || !!definition.readiness,
    readiness: definition.readiness ?? {
      any: definition.ready ? [{ value: definition.ready, regex: false }] : [],
    },
  };
}

export function emberDraftIssue(
  draft: EmberDraft,
  section?: "runtime" | "files",
): string | null {
  if (
    (!section || section === "runtime") &&
    draft.waitForReadyMessage &&
    !draft.readiness.any.length
  )
    return i18n.t("emberEditor.needRule");
  const groups = [
    ...(!section || section === "runtime" ? [draft.images] : []),
    ...((!section || section === "runtime") && draft.commands.length > 1
      ? [draft.commands]
      : []),
    ...(!section || section === "files"
      ? draft.files.map((file) => file.values)
      : []),
  ];
  for (const group of groups) {
    const names = group.map((row) => row.name);
    if (names.some((name) => !name.trim()))
      return i18n.t("emberEditor.completeNames");
    if (new Set(names).size !== names.length)
      return i18n.t("emberEditor.uniqueNames");
  }
  return null;
}

export function serializeEmberDraft(draft: EmberDraft): Ember {
  const issue = emberDraftIssue(draft);
  if (issue) throw new Error(issue);
  const definition = structuredClone(draft.definition);
  definition.images = Object.fromEntries(
    draft.images.map(({ name, value }) => [name, value]),
  );
  if (draft.commands.length > 1) {
    definition.startup = "";
    definition.startup_commands = Object.fromEntries(
      draft.commands.map(({ name, value }) => [name, value]),
    );
  } else {
    definition.startup = draft.commands[0]?.value ?? "";
    delete definition.startup_commands;
  }
  delete definition.ready;
  delete definition.readiness;
  const match = draft.readiness.any[0];
  if (
    draft.waitForReadyMessage &&
    draft.readiness.any.length === 1 &&
    match &&
    !match.regex &&
    !match.ignore_case
  ) {
    definition.ready = match.value;
  } else if (draft.waitForReadyMessage && draft.readiness.any.length) {
    definition.readiness = structuredClone(draft.readiness);
  }
  if (draft.files.length || definition.configs !== undefined) {
    definition.configs = draft.files.map(({ file, values }) => ({
      ...file,
      values: Object.fromEntries(
        values.map(({ name, value }) => [name, value]),
      ),
    }));
  }
  return definition;
}

export function optionalValue<T extends object, K extends keyof T>(
  value: T,
  key: K,
  next: T[K] | undefined,
): T {
  const result = { ...value };
  if (next === undefined) delete result[key];
  else result[key] = next;
  return result;
}
