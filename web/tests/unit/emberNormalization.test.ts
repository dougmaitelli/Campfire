import { expect, test } from "vitest";
import {
  CURRENT_EMBER_SCHEMA,
  normalizeEmber,
} from "../../src/contracts/ember";
import { NetworkProtocol } from "../../src/contracts/domains";
import { completeEmber } from "../fixtures/ember";
import { isEmber } from "../../src/api/embers";

test("every supported schema normalizes to the current version", () => {
  for (let schema = 1; schema <= CURRENT_EMBER_SCHEMA; schema++) {
    const source = {
      ...completeEmber(),
      schema,
      required_ports: [{ container: 25565, protocol: NetworkProtocol.TCP }],
    };
    expect(isEmber(source)).toBe(true);
    const normalized = normalizeEmber(source);
    expect(normalized.schema).toBe(CURRENT_EMBER_SCHEMA);
    expect(normalized.required_ports?.[0]?.primary === true).toBe(schema === 1);
    expect(source.required_ports[0]).not.toHaveProperty("primary");
    expect(normalizeEmber(normalized)).toEqual(normalized);
  }
});

test.each([0, -1, CURRENT_EMBER_SCHEMA + 1])(
  "unsupported schema %s is rejected by the reader boundary",
  (schema) => {
    const source = { ...completeEmber(), schema };
    expect(isEmber(source)).toBe(false);
  },
);
