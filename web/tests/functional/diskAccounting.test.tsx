import { expect, test, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import {
  ConnectionState,
  DiskAccounting,
  RuntimeState,
} from "../../src/contracts/domains";
import { isResourceUsage, type ResourceUsage } from "../../src/api/monitoring";
import { ResourceTiles } from "../../src/components/metrics/ResourceTiles";
import { SparksTable } from "../../src/components/sparks/SparksTable";
import type { Spark } from "../../src/api/sparks";

const usage: ResourceUsage = {
  server_id: "game",
  state: RuntimeState.Running,
  cpu_percent: 10,
  memory_bytes: 1024,
  disk_bytes: 8192,
  sampled_at: 100,
  disk_sampled_at: 100,
  sample_age_seconds: 0,
  disk_age_seconds: 0,
  container_error: "",
  disk_error: "",
  disk_quota_enforced: true,
  disk_quota_error: "",
};
const options = { cpuLimit: 1, memoryMB: 8, diskMB: 8, elapsed: 0 };

test("accounting metadata is optional for older Sparks and validated when present", () => {
  expect(isResourceUsage(usage)).toBe(true);
  for (const method of Object.values(DiskAccounting)) {
    expect(isResourceUsage({ ...usage, disk_accounting: method })).toBe(true);
  }
  for (const method of [null, "", "allocated", 1, {}]) {
    expect(isResourceUsage({ ...usage, disk_accounting: method })).toBe(false);
  }
});

test("current disk readings distinguish native counters, scans, and mixed totals", () => {
  const view = render(<ResourceTiles {...options} usage={usage} />);
  expect(
    screen.getByTitle(/Logical game-data file sizes from a bounded scan/),
  ).toBeDefined();
  expect(screen.getByText("8 KiB")).toBeDefined();
  view.rerender(
    <ResourceTiles
      {...options}
      usage={{ ...usage, disk_accounting: DiskAccounting.Project }}
    />,
  );
  expect(screen.getByTitle(/Filesystem project-quota usage/)).toBeDefined();
  view.rerender(
    <ResourceTiles
      {...options}
      usage={{ ...usage, disk_accounting: DiskAccounting.Mixed }}
    />,
  );
  expect(screen.getByTitle(/Not total host usage/)).toBeDefined();
});

test.each([
  [undefined, /Logical game-data file sizes from a bounded scan/],
  [DiskAccounting.Project, /Filesystem project-quota usage/],
  [DiskAccounting.Mixed, /project-quota counters and logical file-size scans/],
] as const)(
  "Spark table preserves context, accounting (%s), and errors",
  (method, description) => {
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
    const current: ResourceUsage = {
      ...usage,
      server_id: "",
      state: ConnectionState.Connected,
      ...(method ? { disk_accounting: method } : {}),
    };
    const table = (reading: ResourceUsage) => (
      <SparksTable
        sparks={[spark]}
        monitoring={[{ ...spark, usage: reading }]}
        elapsed={0}
        loading={false}
        error={null}
        updating={null}
        onDetails={vi.fn()}
        onRetry={vi.fn()}
        onAdd={vi.fn()}
        onUpdate={vi.fn()}
      />
    );
    const view = render(table(current));
    const reading = screen
      .getByText("Game data", { exact: true })
      .closest(".usage-reading")!;
    expect(reading.getAttribute("title")).toContain(
      "Combined game-server usage against this Spark's configured limit.",
    );
    expect(reading.getAttribute("title")).toMatch(description);
    expect(reading.textContent).toContain("8 KiB");
    expect(reading.querySelector("meter")).not.toBeNull();

    view.rerender(
      table({
        ...current,
        disk_error: "Some game-data measurements are unavailable.",
        disk_quota_error: "Configured allocation is not enforced.",
        disk_quota_enforced: false,
      }),
    );
    expect(reading.getAttribute("title")).toContain(
      "Some game-data measurements are unavailable.",
    );
    expect(reading.getAttribute("title")).toContain(
      "Configured allocation is not enforced.",
    );
    expect(reading.getAttribute("title")).toContain(
      "Combined game-server usage against this Spark's configured limit.",
    );
    expect(reading.getAttribute("title")).toMatch(description);
    expect(reading.textContent).toContain("8 KiB");
    expect(reading.classList.contains("usage-stale")).toBe(true);
    expect(reading.textContent).toContain("stale");
    expect(reading.querySelector("meter")).toBeNull();
  },
);

test("failed native readings retain their units and value but are visibly stale", () => {
  render(
    <ResourceTiles
      {...options}
      usage={{
        ...usage,
        disk_accounting: DiskAccounting.Project,
        disk_error: "Counter unavailable; last reading may be stale.",
        disk_quota_error: "Configured allocation is not enforced.",
        disk_quota_enforced: false,
      }}
    />,
  );
  const reading = screen.getByTitle(
    /Counter unavailable.*Configured allocation is not enforced.*Filesystem project-quota/,
  );
  expect(reading.classList.contains("usage-stale")).toBe(true);
  expect(reading.textContent).toContain("8 KiB");
  expect(reading.textContent).toContain("stale");
  expect(reading.querySelector("meter")).toBeNull();
});
