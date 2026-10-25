import { expect, test } from "vitest";
import { render, screen } from "@testing-library/react";
import { PortGroups } from "../../src/components/network/PortGroups";
import { GameArt } from "../../src/components/media/GameArt";
import { ResourceReadings } from "../../src/components/servers/ResourceReadings";
import { OverviewEmptyState } from "../../src/components/overview/OverviewEmptyState";
import { ServerState } from "../../src/components/servers/ServerState";

test("network groups keep addresses separate and preserve port mappings", () => {
  const view = render(
    <PortGroups
      ports={[
        {
          address: "::1",
          port: 2456,
          protocol: "udp",
          targetPort: 3000,
          description: "Query",
        },
        { address: "::1", port: "2457–2458", protocol: "udp" },
        { address: "127.0.0.1", port: 8080, protocol: "tcp" },
      ]}
    />,
  );
  expect(screen.getAllByText("::1")).toHaveLength(1);
  expect(screen.getByText("127.0.0.1")).toBeDefined();
  expect(screen.getByText("2456 UDP")).toBeDefined();
  expect(screen.getByText("2457–2458 UDP")).toBeDefined();
  expect(screen.getByText("(Query)")).toBeDefined();
  expect(screen.getByText(/3000 in container/)).toBeDefined();
  expect(screen.queryByText("::1:2456")).toBeNull();
  view.rerender(<PortGroups ports={[]} />);
  expect(view.container.textContent).toBe("-");
});

test("game art is optional and uses safe image attributes", () => {
  const view = render(<GameArt />);
  expect(view.container.childElementCount).toBe(0);
  view.rerender(<GameArt src="https://cdn.example.test/game.png" />);
  const image = view.container.querySelector("img");
  expect(image?.getAttribute("src")).toBe("https://cdn.example.test/game.png");
  expect(image?.getAttribute("alt")).toBe("");
  expect(image?.getAttribute("loading")).toBe("lazy");
  expect(image?.getAttribute("referrerpolicy")).toBe("no-referrer");
});

test("resource columns display percentages and allocation ratios accessibly", () => {
  render(
    <table>
      <tbody>
        <tr>
          <ResourceReadings
            usage={{
              server_id: "one",
              state: "running",
              cpu_percent: 150,
              memory_bytes: 1048576,
              disk_bytes: 2097152,
              sampled_at: 100,
              disk_sampled_at: 100,
              sample_age_seconds: 0,
              disk_age_seconds: 0,
              container_error: "",
              disk_error: "",
              disk_quota_enforced: false,
              disk_quota_error: "",
            }}
            cpuLimit={2}
            memoryMB={4}
            diskMB={8}
            elapsed={0}
            layout="columns"
          />
        </tr>
      </tbody>
    </table>,
  );
  const cells = screen.getAllByRole("cell");
  expect(cells).toHaveLength(3);
  expect(cells[0]?.textContent).toContain("75%");
  expect(cells[0]?.querySelector("meter")?.getAttribute("value")).toBe("0.75");
  expect(cells[1]?.textContent).toContain("1 MiB");
  expect(cells[1]?.textContent).toContain("4 MiB");
  for (const cell of cells)
    expect(cell.querySelector(".visually-hidden")).not.toBeNull();
});

test("overview setup reflects existing Sparks and management permissions", () => {
  const view = render(
    <OverviewEmptyState
      sparkCount={0}
      canManageSparks
      canManageEmbers
      canCreateServers
    />,
  );
  expect(
    screen.getByRole("heading", { name: "Start your campfire" }),
  ).toBeDefined();
  expect(screen.getByRole("link", { name: /Add Spark/ })).toBeDefined();
  expect(screen.queryByText("Complete")).toBeNull();
  expect(
    screen.getByRole("link", { name: "Import an Ember" }).getAttribute("href"),
  ).toBe("#embers");
  view.rerender(
    <OverviewEmptyState
      sparkCount={1}
      canManageSparks
      canManageEmbers
      canCreateServers
    />,
  );
  expect(screen.getByText("Complete")).toBeDefined();
  expect(screen.getByRole("link", { name: /View Sparks/ })).toBeDefined();
  expect(screen.queryByRole("link", { name: /Add Spark/ })).toBeNull();
  view.rerender(
    <OverviewEmptyState
      sparkCount={undefined}
      canManageSparks={false}
      canManageEmbers={false}
      canCreateServers={false}
    />,
  );
  expect(screen.getByText(/Ask an administrator/)).toBeDefined();
  expect(screen.queryByRole("link")).toBeNull();
});

test("server timing uses observed timestamps and hides unknown or future times", () => {
  const view = render(
    <ServerState now={200000} state="running" startedAt={192740} />,
  );
  expect(screen.getByText("Uptime 2h 1m")).toBeDefined();
  view.rerender(
    <ServerState now={200000} state="stopped" stoppedAt={199400} />,
  );
  expect(screen.getByText("Stopped 10m ago")).toBeDefined();
  for (const props of [
    { state: "running" },
    { state: "unknown", stoppedAt: 199400 },
    { state: "running", startedAt: 300000 },
  ]) {
    view.rerender(<ServerState now={200000} {...props} />);
    expect(view.container.querySelector(".server-state-timing")).toBeNull();
  }
});
