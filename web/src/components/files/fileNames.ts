// Linux files and directories use the same entry-name rules.
export function normalizeFileName(value: string): string | null {
  const name = value.trim();
  return !name ||
    name === "." ||
    name === ".." ||
    name.includes("/") ||
    name.includes("\\") ||
    /\p{Cc}/u.test(name)
    ? null
    : name;
}

export function availableFileName(name: string, reserved: ReadonlySet<string>) {
  name = name.trim() || "file";
  const compound = /\.tar\.(?:gz|bz2|xz|lz|lz4|zst|zstd)$/i.exec(name);
  const dot = compound?.index ?? name.lastIndexOf(".");
  const base = dot > 0 ? name.slice(0, dot) : name;
  const extension = dot > 0 ? name.slice(dot) : "";
  let candidate = `${base} copy${extension}`;
  for (let index = 2; reserved.has(candidate); index++)
    candidate = `${base} copy ${index}${extension}`;
  return candidate;
}
