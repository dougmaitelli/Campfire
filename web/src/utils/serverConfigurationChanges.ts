import type { ServerConfiguration } from "../api/servers";

export type ConfigurationChange = "added" | "changed" | "removed";
export interface ServerConfigurationChanges {
  fields: Partial<
    Record<Exclude<keyof ServerConfiguration, "variables">, "changed">
  >;
  variables: Record<string, ConfigurationChange>;
}

// Compare saved server values only; inherited Ember defaults are not a baseline.
export function serverConfigurationChanges(
  before: ServerConfiguration,
  after: ServerConfiguration,
): ServerConfigurationChanges {
  const fields: ServerConfigurationChanges["fields"] = {};
  for (const key of [
    "name",
    "description",
    "spark_id",
    "ember_id",
    "image",
    "cpu_limit",
    "memory_mb",
    "disk_mb",
  ] as const) {
    if (before[key] !== after[key]) fields[key] = "changed";
  }
  for (const key of ["startup_command", "startup_override"] as const) {
    if ((before[key] ?? "") !== (after[key] ?? "")) fields[key] = "changed";
  }
  if ((before.game_integration ?? false) !== (after.game_integration ?? false))
    fields.game_integration = "changed";
  const ports = (value: ServerConfiguration) =>
    value.ports.map((port) => [
      port.bind_ip,
      port.host_port,
      port.container_port,
      port.protocol,
    ]);
  if (JSON.stringify(ports(before)) !== JSON.stringify(ports(after)))
    fields.ports = "changed";

  const variables: Record<string, ConfigurationChange> = {};
  for (const name of new Set([
    ...Object.keys(before.variables),
    ...Object.keys(after.variables),
  ])) {
    if (!Object.hasOwn(after.variables, name)) variables[name] = "removed";
    else if (!Object.hasOwn(before.variables, name)) variables[name] = "added";
    else if (before.variables[name] !== after.variables[name])
      variables[name] = "changed";
  }
  return { fields, variables };
}
