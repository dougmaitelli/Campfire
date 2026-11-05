import { NetworkProtocol } from "../contracts/domains";
import type { Ember } from "../api/embers";
import type { Server, ServerPort } from "../api/servers";
import type { Spark } from "../api/sparks";

export function defaultPorts(
  spark: Spark | undefined,
  ember: Ember | undefined,
  servers: Server[],
  excludedID?: string,
  mappings?: ServerPort[],
): ServerPort[] {
  if (!spark || (!ember && !mappings)) return [];
  const used = servers
    .filter(
      (server) => server.spark_id === spark.id && server.id !== excludedID,
    )
    .flatMap((server) => server.ports);
  const required = mappings
    ? mappings.map((port) => ({
        container: port.container_port,
        protocol: port.protocol,
      }))
    : ember?.required_ports?.length
      ? ember.required_ports
      : [
          {
            container: 25565,
            protocol:
              spark.port_allocations[0]?.protocol ?? NetworkProtocol.TCP,
          },
        ];
  const result: ServerPort[] = [];
  for (const port of required) {
    for (const allocation of spark.port_allocations) {
      if (allocation.protocol !== port.protocol) continue;
      let host = allocation.start;
      while (
        host <= allocation.end &&
        used.some(
          (port) =>
            port.host_port === host &&
            port.protocol === allocation.protocol &&
            (port.bind_ip === allocation.bind_ip ||
              (port.bind_ip.includes(":") ===
                allocation.bind_ip.includes(":") &&
                [port.bind_ip, allocation.bind_ip].some(
                  (ip) => ip === "0.0.0.0" || ip === "::",
                ))),
        )
      )
        host++;
      if (host > allocation.end) continue;
      const mapping = {
        bind_ip: allocation.bind_ip,
        host_port: host,
        container_port: port.container,
        protocol: allocation.protocol,
      };
      result.push(mapping);
      used.push(mapping);
      break;
    }
  }
  return result;
}

export function defaultVariables(
  ember: Ember | undefined,
): Record<string, string> {
  return Object.fromEntries(
    (ember?.variables ?? [])
      .filter((variable) => !variable.hidden)
      .map((variable) => [variable.name, variable.default]),
  );
}

// Copies preserve every container mapping, including custom and empty lists.
export function copyPorts(
  spark: Spark | undefined,
  ports: ServerPort[],
  servers: Server[],
): ServerPort[] {
  if (ports.length === 0) return [];
  return defaultPorts(spark, undefined, servers, undefined, ports);
}
