import { describe, expect, it } from "vitest";
import {
  archiveFolderName,
  isExtractableArchive,
} from "../../src/components/files/archiveFormats";

describe("server-file archive names", () => {
  it.each([
    "zip",
    "rar",
    "tar",
    "tar.gz",
    "tgz",
    "tar.xz",
    "txz",
    "tar.lz",
    "tlz",
    "tar.bz2",
    "tbz",
    "tbz2",
    "tar.lz4",
    "tar.zst",
    "tar.zstd",
    "tzst",
  ])("recognizes %s and removes the whole suffix", (extension) => {
    expect(isExtractableArchive(`world.${extension.toUpperCase()}`)).toBe(true);
    expect(archiveFolderName(`world.${extension.toUpperCase()}`)).toBe("world");
  });
  it.each([
    "world.7z",
    "world.gz",
    "world.xz",
    "world.zip.001",
    "world.tar.gz.exe",
  ])("does not advertise unsupported %s", (name) => {
    expect(isExtractableArchive(name)).toBe(false);
    expect(archiveFolderName(name)).toBe(name);
  });
  it("does not propose an empty extraction folder", () => {
    expect(archiveFolderName(".tar.xz")).toBe("extracted");
  });
});
