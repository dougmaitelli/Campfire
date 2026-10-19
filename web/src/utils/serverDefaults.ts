import type { Ember } from "../api/embers";
import type { Server, ServerPort } from "../api/servers";
import type { Spark } from "../api/sparks";

export function defaultPorts(
  spark: Spark | undefined,
  ember: Ember | undefined,
  servers: Server[],
  excludedID?: string,
): ServerPort[] {
  if (!spark || !ember) return [];
  const used = new Set(
    servers
      .filter(
        (server) => server.spark_id === spark.id && server.id !== excludedID,
      )
      .flatMap((server) =>
        server.ports.map(
          (port) => `${port.bind_ip}/${port.host_port}/${port.protocol}`,
        ),
      ),
  );
  const required = ember.required_ports?.length
    ? ember.required_ports
    : [
        {
          container: 25565,
          protocol: spark.port_allocations[0]?.protocol ?? "tcp",
        },
      ];
  const result: ServerPort[] = [];
  for (const port of required) {
    for (const allocation of spark.port_allocations) {
      if (allocation.protocol !== port.protocol) continue;
      let host = allocation.start;
      while (
        host <= allocation.end &&
        used.has(`${allocation.bind_ip}/${host}/${allocation.protocol}`)
      )
        host++;
      if (host > allocation.end) continue;
      result.push({
        bind_ip: allocation.bind_ip,
        host_port: host,
        container_port: port.container,
        protocol: allocation.protocol,
      });
      used.add(`${allocation.bind_ip}/${host}/${allocation.protocol}`);
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
