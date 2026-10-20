// Keep release identifiers intact; development builds carry a full Git SHA.
export function formatVersion(version: string): string {
  return version.replace(
    /^(dev-)([a-f\d]{9,40})$/i,
    (_match, prefix: string, sha: string) => `${prefix}${sha.slice(0, 8)}…`,
  );
}
