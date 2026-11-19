import { expect, expectTypeOf, test } from "vitest";
import * as domains from "../../src/contracts/domains";
import { Capability } from "../../src/contracts/capabilities";
import { Permission } from "../../src/contracts/permissions";
import {
  LogAction,
  LogPhase,
  LogSeverity,
} from "../../src/contracts/logAlerts";
import type { PublicationStatus } from "../../src/api/fileTypes";
import { isEnumValue } from "../../src/utils/validation";

const catalog = {
  ...domains,
  Capability,
  Permission,
  LogAction,
  LogPhase,
  LogSeverity,
};

// Check every member, including subset/composite views. Reverting even one
// member to an ordinary string literal makes the typecheck fail.
type RawStringMembers<T extends Record<string, string>> = {
  [K in keyof T]: `${T[K]}` extends T[K] ? K : never;
}[keyof T];
type DomainsAcceptingRawStrings = {
  [K in keyof typeof catalog]: RawStringMembers<
    (typeof catalog)[K]
  > extends never
    ? never
    : K;
}[keyof typeof catalog];

test("closed domains require enum members, including subsets and combined outcomes", () => {
  expectTypeOf<DomainsAcceptingRawStrings>().toEqualTypeOf<never>();
  expectTypeOf<"start">().not.toMatchTypeOf<domains.ScheduleAction>();
  expectTypeOf<"connected">().not.toMatchTypeOf<domains.ResourceState>();
  expectTypeOf<"uploaded">().not.toMatchTypeOf<PublicationStatus>();
  expectTypeOf<"unconfirmed">().not.toMatchTypeOf<PublicationStatus>();
  expectTypeOf<"extracted">().not.toMatchTypeOf<PublicationStatus>();
  expectTypeOf<domains.CompressionState.Running>().not.toMatchTypeOf<domains.ExtractionState>();
});

test.each(Object.entries(catalog))(
  "%s validates only its string values at runtime",
  (_name, domain) => {
    const values = Object.values(domain);
    expect(values.length).toBeGreaterThan(0);
    expect(new Set(values).size).toBe(values.length);
    for (const value of values) {
      expect(typeof value).toBe("string");
      expect(isEnumValue<string>(domain, value)).toBe(true);
    }
    for (const invalid of [
      undefined,
      null,
      0,
      {},
      [],
      [values[0]],
      "__unknown_enum_value__",
    ])
      expect(isEnumValue<string>(domain, invalid)).toBe(false);
  },
);

test("domain members preserve serialized contract values", () => {
  expect(JSON.parse(JSON.stringify(catalog))).toMatchSnapshot();
});
