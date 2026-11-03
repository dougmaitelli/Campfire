import { expect, test } from "vitest";
import { isEmber } from "../../src/api/embers";
import { completeEmber } from "../fixtures/ember";

test("Ember responses accept the full editor model and nullable server fields", () => {
  const ember = completeEmber();
  expect(isEmber(ember)).toBe(true);
  ember.variables = null;
  ember.required_ports = null;
  ember.configs![0]!.values = null;
  ember.origin!.warnings = null;
  expect(isEmber(ember)).toBe(true);
});

test.each([
  { installer: { image: "alpine", entrypoint: "sh", script: null } },
  { stop: false },
  { ready: 1 },
  { readiness: { any: null } },
  { readiness: { any: [{ value: "Ready", regex: "yes" }] } },
  { readiness: { any: [{ value: "Ready", ignore_case: "yes" }] } },
  { configs: [{ path: "settings.json", parser: "xml", values: {} }] },
  { configs: [{ path: "settings.json", parser: "json", values: [] }] },
  { configs: [{ path: "settings.json", parser: "json", values: { port: 1 } }] },
  { configs: [null] },
  { savedata_paths: [{ path: "world", required: "yes" }] },
  { origin: { format: "PLCN_v3", author: "", update_url: "", warnings: "" } },
])("Ember responses reject malformed editor fields: %j", (fields) => {
  expect(isEmber({ ...completeEmber(), ...fields })).toBe(false);
});

test.each([
  { pattern: true },
  { validation: { type: "string", min_length: "1" } },
  { validation: { type: "string", max_length: -1 } },
  { validation: { type: "integer", min: "1" } },
  { validation: { type: "numeric", max: Infinity } },
  { validation: { type: "string", nullable: "yes" } },
  { validation: { type: "string", choices: [1] } },
])("Ember responses reject malformed variable rules: %j", (fields) => {
  const ember = completeEmber();
  expect(
    isEmber({ ...ember, variables: [{ ...ember.variables![0], ...fields }] }),
  ).toBe(false);
});
