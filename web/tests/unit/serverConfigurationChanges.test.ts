import { expect, test } from "vitest";
import type { ServerConfiguration } from "../../src/api/servers";
import { serverConfigurationChanges } from "../../src/utils/serverConfigurationChanges";

const saved: ServerConfiguration = {
  name: "Survival",
  description: "",
  spark_id: "spark",
  ember_id: "ember",
  image: "game:old",
  cpu_limit: 2,
  memory_mb: 4096,
  disk_mb: 20480,
  variables: { RETAINED: "custom", LOCKED: "old", REMOVED: "value", EMPTY: "" },
  ports: [
    {
      bind_ip: "0.0.0.0",
      host_port: 25565,
      container_port: 25565,
      protocol: "tcp",
    },
  ],
};

test("server comparison distinguishes changed, added-empty, retained, and removed variables", () => {
  const changes = serverConfigurationChanges(saved, {
    ...saved,
    image: "game:new",
    variables: { RETAINED: "custom", LOCKED: "new", EMPTY: "", ADDED: "" },
  });
  expect(changes.fields).toEqual({ image: "changed" });
  expect(changes.variables).toEqual({
    LOCKED: "changed",
    REMOVED: "removed",
    ADDED: "added",
  });
});

test("unset optional overrides and equivalent port mappings do not create changes", () => {
  expect(
    serverConfigurationChanges(saved, {
      ...saved,
      startup_override: "",
      startup_command: "",
      game_integration: false,
    }),
  ).toEqual({ fields: {}, variables: {} });
  const other = {
    bind_ip: "127.0.0.1",
    host_port: 27015,
    container_port: 27015,
    protocol: "udp",
  } as const;
  expect(
    serverConfigurationChanges(
      { ...saved, ports: [...saved.ports, other] },
      { ...saved, ports: [other, ...saved.ports] },
    ).fields,
  ).toEqual({});
  expect(
    serverConfigurationChanges(saved, { ...saved, ports: [] }).fields,
  ).toEqual({ ports: "changed" });
});

test("setting or clearing a server startup override is compared with the saved override", () => {
  expect(
    serverConfigurationChanges(saved, {
      ...saved,
      startup_override: "custom command",
    }).fields,
  ).toEqual({ startup_override: "changed" });
  expect(
    serverConfigurationChanges(
      { ...saved, startup_override: "custom command" },
      saved,
    ).fields,
  ).toEqual({ startup_override: "changed" });
  expect(serverConfigurationChanges(saved, saved)).toEqual({
    fields: {},
    variables: {},
  });
});
