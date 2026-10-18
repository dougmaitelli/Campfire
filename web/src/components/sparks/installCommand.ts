function quote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

export function validAgentAddress(value: string): boolean {
  try {
    const url = new URL(value);
    return (
      url.protocol === "https:" &&
      !url.username &&
      !url.password &&
      url.pathname === "/" &&
      !url.search &&
      !url.hash
    );
  } catch {
    return false;
  }
}

export function sparkInstallCommand(
  origin: string,
  panel: string,
  token: string,
  caFile: string,
): string {
  if (!validAgentAddress(panel)) return "";
  const script = `set -eu; installer=$(mktemp); trap 'rm -f -- "$installer"' EXIT; curl --fail --silent --show-error --location ${quote(`${origin}/install-spark.sh`)} --output "$installer"; sudo bash "$installer" --panel ${quote(panel)} --token ${quote(token)}${caFile.trim() ? ` --ca-file ${quote(caFile.trim())}` : ""}`;
  return `bash -c ${quote(script)}`;
}
