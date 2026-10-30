import { expect, test } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { SparkFeatures } from "../../src/components/sparks/SparkFeatures";
import type { Spark } from "../../src/api/sparks";

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
};

test("Spark features translate known capabilities and preserve unmapped keys", () => {
  const capabilities = [
    "server.game-media.v1",
    "server.future-feature.v1",
    "constructor",
  ];
  const view = render(<SparkFeatures spark={{ ...spark, capabilities }} />);
  expect(screen.getByText("Game screenshots and clips")).toBeDefined();
  expect(screen.getByText("server.future-feature.v1")).toBeDefined();
  expect(screen.getByText("constructor")).toBeDefined();
  expect(
    screen.getByText("server.future-feature.v1").closest("li")?.textContent,
  ).toContain("Available");
  view.rerender(
    <SparkFeatures spark={{ ...spark, capabilities, connected: false }} />,
  );
  expect(screen.getByText("server.future-feature.v1")).toBeDefined();
  expect(
    screen.getByText("server.future-feature.v1").closest("li")?.textContent,
  ).toContain("Unknown");
});

test("merged features require every capability and count supported groups", () => {
  const base = [
    "server.files.v1",
    "spark.self-update.v1",
    "server.game-integration.v1",
  ];
  const extensions = [
    "server.files.permissions.v1",
    "spark.self-reinstall.v1",
    "server.game-events.v1",
  ];
  const labels = [
    "File management",
    "Remote Spark updates",
    "Server state and event publishing",
  ];
  const view = render(
    <SparkFeatures spark={{ ...spark, capabilities: base }} />,
  );
  function expectState(state: string) {
    for (const label of labels) {
      expect(
        within(screen.getByText(label).closest("li")!).getByText(state),
      ).toBeDefined();
    }
    expect(screen.getAllByRole("listitem")).toHaveLength(17);
    for (const capability of extensions) {
      expect(screen.queryByText(capability)).toBeNull();
    }
  }
  expectState("Partial support");
  expect(
    screen.getByRole("heading", { name: "Features (0/17)" }),
  ).toBeDefined();

  view.rerender(
    <SparkFeatures
      spark={{ ...spark, capabilities: [...base, ...extensions] }}
    />,
  );
  expectState("Available");
  expect(
    screen.getByRole("heading", { name: "Features (3/17)" }),
  ).toBeDefined();

  view.rerender(
    <SparkFeatures spark={{ ...spark, capabilities: extensions }} />,
  );
  expectState("Partial support");
  expect(
    screen.getByRole("heading", { name: "Features (0/17)" }),
  ).toBeDefined();

  view.rerender(<SparkFeatures spark={{ ...spark, capabilities: [] }} />);
  expectState("Unavailable");

  view.rerender(
    <SparkFeatures
      spark={{
        ...spark,
        capabilities: [...base, ...extensions],
        connected: false,
      }}
    />,
  );
  expectState("Unknown");
  expect(screen.getByRole("heading", { name: "Features" })).toBeDefined();
});
