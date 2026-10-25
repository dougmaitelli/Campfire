import { expect, test } from "vitest";
import { render, screen } from "@testing-library/react";
import { SparkFeatures } from "../../src/components/sparks/SparkFeatures";
import type { Spark } from "../../src/api/sparks";

test("Spark features translate known capabilities and preserve unmapped keys", () => {
  const spark: Spark = {
    id: "node",
    name: "Node",
    description: "",
    disabled: false,
    connected: true,
    game_address: "127.0.0.1",
    cpu_limit: 1,
    memory_mb: 1024,
    disk_mb: 4096,
    port_allocations: [],
    created_at: "2026-10-24T00:00:00Z",
    capabilities: [
      "server.game-media.v1",
      "server.future-feature.v1",
      "constructor",
    ],
  };
  const view = render(<SparkFeatures spark={spark} />);
  expect(screen.getByText("Game screenshots and clips")).toBeDefined();
  expect(screen.getByText("server.future-feature.v1")).toBeDefined();
  expect(screen.getByText("constructor")).toBeDefined();
  expect(
    screen.getByText("server.future-feature.v1").closest("li")?.textContent,
  ).toContain("Available");
  view.rerender(<SparkFeatures spark={{ ...spark, connected: false }} />);
  expect(screen.getByText("server.future-feature.v1")).toBeDefined();
  expect(
    screen.getByText("server.future-feature.v1").closest("li")?.textContent,
  ).toContain("Unknown");
});
