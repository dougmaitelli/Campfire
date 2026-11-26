// Keep recognition and default extraction-folder names aligned. Standalone
// compressed streams and split archives are not server-file archives.
const archiveSuffix =
  /\.(?:tar\.(?:gz|xz|lz|bz2|lz4|zst|zstd)|tgz|txz|tlz|tbz2?|tzst|tar|zip|rar)$/i;

export function isExtractableArchive(name: string): boolean {
  return archiveSuffix.test(name);
}

export function archiveFolderName(name: string): string {
  return name.replace(archiveSuffix, "") || "extracted";
}
