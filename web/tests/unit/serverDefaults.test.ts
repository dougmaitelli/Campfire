import { expect, test } from "vitest";
import type { Spark } from "../../src/api/sparks";
import type { ServerPort } from "../../src/api/servers";
import { NetworkProtocol } from "../../src/contracts/domains";
import { emptyEmber } from "../../src/components/embers/emberDraft";
import { copyPorts, defaultPorts } from "../../src/utils/serverDefaults";

const spark: Spark = {
  id: "node",
  name: "Node",
  description: "",
  disabled: false,
  connected: true,
  game_address: "games.example.test",
  cpu_limit: 4,
  memory_mb: 4096,
  disk_mb: 8192,
  port_allocations: [
    {
      bind_ip: "0.0.0.0",
      start: 30000,
      end: 30010,
      protocol: NetworkProtocol.TCP,
    },
  ],
  created_at: "2026-01-01T00:00:00Z",
};

for (const schema of [1, 2]) {
  test(`schema ${schema} does not invent mappings for absent, null, or empty port requirements`, () => {
    const ember = { ...emptyEmber(), schema };
    expect(defaultPorts(spark, ember, [])).toEqual([]);
    ember.required_ports = null;
    expect(defaultPorts(spark, ember, [])).toEqual([]);
    ember.required_ports = [];
    expect(defaultPorts(spark, ember, [])).toEqual([]);
  });
}

test("declared ports are allocated independently of primary designation", () => {
  const ember = emptyEmber();
  ember.required_ports = [
    { container: 28015, protocol: NetworkProtocol.TCP },
    { container: 28016, protocol: NetworkProtocol.TCP, primary: true },
  ];
  expect(defaultPorts(spark, ember, [])).toEqual([
    {
      bind_ip: "0.0.0.0",
      host_port: 30000,
      container_port: 28015,
      protocol: NetworkProtocol.TCP,
    },
    {
      bind_ip: "0.0.0.0",
      host_port: 30001,
      container_port: 28016,
      protocol: NetworkProtocol.TCP,
    },
  ]);
});

test("copies preserve explicit custom mappings and empty port lists", () => {
  const ports: ServerPort[] = [
    {
      bind_ip: "192.0.2.1",
      host_port: 31000,
      container_port: 12345,
      protocol: NetworkProtocol.TCP,
    },
  ];
  expect(copyPorts(spark, ports, [])).toEqual([
    {
      bind_ip: "0.0.0.0",
      host_port: 30000,
      container_port: 12345,
      protocol: NetworkProtocol.TCP,
    },
  ]);
  expect(ports[0]?.host_port).toBe(31000);
  expect(copyPorts(spark, [], [])).toEqual([]);
  const ember = emptyEmber();
  ember.required_ports = [{ container: 28015, protocol: NetworkProtocol.TCP }];
  expect(defaultPorts(spark, ember, [], undefined, [])).toEqual([]);
});
