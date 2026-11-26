import type { ServerPort } from "../api/servers";
import type { Ember } from "../contracts/ember";

// Mirrors the panel's display-only resolver. Never infer a public host from bind IPs.
export function resolveGameAddress(
  override: string | undefined,
  host: string | undefined,
  port: number,
): string {
  if (override) return override;
  if (
    !host ||
    !usableDefaultHost(host) ||
    !Number.isInteger(port) ||
    port < 1 ||
    port > 65535
  )
    return "";
  return `${host.includes(":") ? `[${host}]` : host}:${port}`;
}

// Hostnames (including LAN names) remain allowed. Normalize legacy IPv6
// spellings locally; constructing a URL does not perform a network request.
function usableDefaultHost(host: string): boolean {
  if (host.includes(":")) {
    let ip: string;
    try {
      ip = new URL(`http://[${host}]/`).hostname.slice(1, -1);
    } catch {
      return false;
    }
    if (ip === "::" || /^ff[0-9a-f]{2}:/.test(ip)) return false;
    const mapped = /^::ffff:([0-9a-f]+):([0-9a-f]+)$/.exec(ip);
    if (mapped) {
      const high = Number.parseInt(mapped[1]!, 16);
      const low = Number.parseInt(mapped[2]!, 16);
      const first = high >>> 8;
      return !(high === 0 && low === 0) && !(first >= 224 && first <= 239);
    }
    return true;
  }
  const octets = host.split(".");
  if (
    octets.length === 4 &&
    octets.every(
      (part) => /^(0|[1-9]\d{0,2})$/.test(part) && Number(part) <= 255,
    )
  ) {
    const first = Number(octets[0]);
    return host !== "0.0.0.0" && !(first >= 224 && first <= 239);
  }
  return true;
}

// Preview the shared model's explicit designation, never array order.
export function primaryGamePort(
  ports: ServerPort[],
  required: Ember["required_ports"],
): number {
  const primary = required?.find((port) => port.primary);
  if (!primary) return 0;
  let result = 0;
  for (const port of ports) {
    if (
      port.container_port !== primary.container ||
      port.protocol !== primary.protocol
    )
      continue;
    if (
      !Number.isInteger(port.host_port) ||
      port.host_port < 1 ||
      port.host_port > 65535
    )
      return 0;
    if (result && result !== port.host_port) return 0;
    result = port.host_port;
  }
  return result;
}
