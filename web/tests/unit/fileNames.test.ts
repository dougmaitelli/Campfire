import { expect, test } from "vitest";
import {
  normalizeFileName,
  availableFileName,
} from "../../src/components/files/fileNames";

test.each([
  "",
  "  ",
  ".",
  "..",
  "a/b",
  "a\\b",
  "a\u0000b",
  "a\u007fb",
  "a\u0085b",
])("shared entry-name validation rejects %j", (value) => {
  expect(normalizeFileName(value)).toBeNull();
});
test.each(["config", ".env", "Config folder", "save.tar.gz"])(
  "shared entry-name validation accepts files and folders named %j",
  (value) => {
    expect(normalizeFileName(` ${value} `)).toBe(value);
  },
);
test("unique names share extension handling and never mutate reservations", () => {
  const reserved = new Set(["world copy.tar.gz", "world copy 2.tar.gz"]);
  expect(availableFileName("world.tar.gz", reserved)).toBe(
    "world copy 3.tar.gz",
  );
  expect(availableFileName(".env", reserved)).toBe(".env copy");
  expect([...reserved]).toEqual(["world copy.tar.gz", "world copy 2.tar.gz"]);
});
