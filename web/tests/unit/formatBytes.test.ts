import { expect, test } from "vitest";
import i18n from "../../src/i18n";
import { formatBytes, BYTES_PER_MEBIBYTE } from "../../src/utils/formatBytes";

test.each([
  [0, "0 B"],
  [512, "512 B"],
  [1023, "1,023 B"],
  [1024, "1 KiB"],
  [1536, "1.5 KiB"],
  [BYTES_PER_MEBIBYTE, "1 MiB"],
  [1536 * BYTES_PER_MEBIBYTE, "1.5 GiB"],
  [4096 * BYTES_PER_MEBIBYTE, "4 GiB"],
  [1024 ** 4, "1 TiB"],
  [1024 ** 5, "1 PiB"],
  [1.25 * 1024 ** 3, "1.3 GiB"],
  [Number.NaN, "-"],
  [Number.POSITIVE_INFINITY, "-"],
  [-1, "-"],
])("formats %s bytes as %s", (value, expected) => {
  expect(formatBytes(value)).toBe(expected);
});

test("data sizes use the current translation", async () => {
  await i18n.init({ ...i18n.options, supportedLngs: ["en", "de"] });
  i18n.addResourceBundle("de", "translation", {
    dataSize: { value: "{{unit}}: {{value}}", gib: "translated unit" },
  });
  try {
    await i18n.changeLanguage("de");
    expect(formatBytes(1536 * BYTES_PER_MEBIBYTE)).toBe("translated unit: 1,5");
  } finally {
    i18n.removeResourceBundle("de", "translation");
  }
});
