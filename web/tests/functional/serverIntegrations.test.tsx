import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { Permission } from "../../src/api/permissions";
import type { Server } from "../../src/api/servers";
import {
  readMonitoring,
  type MonitoringSnapshot,
} from "../../src/api/monitoring";
import { IntegrationLimits } from "../../src/config/policies";
import {
  listServerDiscord,
  serverDiscordChannels,
  saveServerIntegrationChanges,
  type ServerIntegrationList,
  type ServerIntegrationSettings,
} from "../../src/api/integrations";
import { useServerIntegrations } from "../../src/hooks/useServerIntegrations";

vi.mock("../../src/api/integrations", () => ({
  listServerDiscord: vi.fn(),
  serverDiscordChannels: vi.fn(),
  saveServerIntegrationChanges: vi.fn(),
}));
vi.mock("../../src/api/monitoring", () => ({ readMonitoring: vi.fn() }));

const server = {
  id: "server",
  permissions: [Permission.ServerView, Permission.ServerConfigure],
} as Server;
const original: ServerIntegrationSettings = {
  id: "existing",
  integration_id: "discord",
  configured: true,
  available: true,
  guild_name: "Community",
  sources: [{ id: "game", permission: Permission.ServerView }],
  enabled: true,
  channel_id: "400",
  subscriptions: [{ category: "game" }],
  card: { enabled: false, fields: [] },
  card_status: {
    state: "disabled",
    error: "",
    next_attempt_at: 0,
    can_retry: false,
  },
};
const overview: ServerIntegrationList = {
  configured: true,
  available: true,
  guild_name: "Community",
  sources: original.sources,
  connections: [
    { id: "discord", provider: "discord", name: "Community", enabled: true },
  ],
  items: [original],
};

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(listServerDiscord).mockResolvedValue(overview);
  vi.mocked(serverDiscordChannels).mockResolvedValue([
    { id: "400", name: "public", available: true, missing: [] },
    { id: "402", name: "admin", available: true, missing: [] },
  ]);
  vi.mocked(saveServerIntegrationChanges).mockImplementation(
    async (_server, changes) =>
      changes.upserts.map((item) => ({
        ...original,
        ...item.settings,
        id: item.id,
      })),
  );
});

test("integration edits remain drafts until the wizard saves and are discarded on close", async () => {
  const { result, unmount } = renderHook(() =>
    useServerIntegrations(server, true),
  );
  await waitFor(() => expect(result.current.items).toHaveLength(1));
  act(() =>
    result.current.update("existing", {
      ...result.current.items[0]!.value,
      enabled: false,
    }),
  );
  act(() => result.current.add());
  expect(result.current.dirty).toBe(true);
  expect(saveServerIntegrationChanges).not.toHaveBeenCalled();
  unmount();
  const reopened = renderHook(() => useServerIntegrations(server, true));
  await waitFor(() => expect(reopened.result.current.items).toHaveLength(1));
  expect(reopened.result.current.items[0]!.value.enabled).toBe(true);
  expect(reopened.result.current.dirty).toBe(false);
});

test("unfinished game rules stay invalid when switching between integration entries", async () => {
  const { result } = renderHook(() => useServerIntegrations(server, true));
  await waitFor(() => expect(result.current.items).toHaveLength(1));
  act(() =>
    result.current.update("existing", result.current.items[0]!.value, [
      { type: null, fields: [{ name: "player", value: "doug" }] },
    ]),
  );
  act(() => result.current.setEditing(null));
  act(() => result.current.setEditing("existing"));
  expect(result.current.items[0]!.rules[0]!.type).toBe(null);
  expect(result.current.validate()).toBe(
    "Complete or remove unfinished event filters.",
  );
  await expect(result.current.save()).rejects.toThrow("Complete or remove");
  expect(saveServerIntegrationChanges).not.toHaveBeenCalled();
});

test("retry after a committed batch loses its response uses the same creation and deletion IDs", async () => {
  const { result } = renderHook(() => useServerIntegrations(server, true));
  await waitFor(() => expect(result.current.channels.discord).toHaveLength(2));
  act(() => result.current.remove("existing"));
  act(() => result.current.add());
  const first = result.current.items[0]!;
  act(() =>
    result.current.update(first.key, { ...first.value, channel_id: "400" }),
  );
  act(() => result.current.add());
  const second = result.current.items[1]!;
  act(() =>
    result.current.update(second.key, { ...second.value, channel_id: "402" }),
  );
  const persisted = new Map([[original.id, original]]);
  let loseResponse = true;
  vi.mocked(saveServerIntegrationChanges).mockImplementation(
    async (_server, changes) => {
      for (const id of changes.remove_ids) persisted.delete(id);
      const values = changes.upserts.map((item) => ({
        ...original,
        ...item.settings,
        id: item.id,
      }));
      for (const item of values) persisted.set(item.id, item);
      if (loseResponse) {
        loseResponse = false;
        throw new Error("Response lost after commit");
      }
      return values;
    },
  );
  await act(async () => {
    await expect(result.current.save()).rejects.toThrow(
      "Response lost after commit",
    );
  });
  expect(persisted.size).toBe(2);
  await act(async () => {
    await result.current.save();
  });
  const calls = vi.mocked(saveServerIntegrationChanges).mock.calls;
  expect(calls).toHaveLength(2);
  expect(calls[0]![1]).toEqual(calls[1]![1]);
  expect(calls[1]![1].remove_ids).toEqual(["existing"]);
  expect(calls[1]![1].upserts.map((item) => item.id)).toEqual([
    first.key,
    second.key,
  ]);
  expect(persisted.size).toBe(2);
  expect(result.current.dirty).toBe(false);
});

test("channel swaps are sent together instead of conflicting sequential writes", async () => {
  vi.mocked(listServerDiscord).mockResolvedValue({
    ...overview,
    items: [original, { ...original, id: "second", channel_id: "402" }],
  });
  const { result } = renderHook(() => useServerIntegrations(server, true));
  await waitFor(() => expect(result.current.channels.discord).toHaveLength(2));
  const [first, second] = result.current.items;
  act(() =>
    result.current.update(first!.key, { ...first!.value, channel_id: "402" }),
  );
  act(() =>
    result.current.update(second!.key, { ...second!.value, channel_id: "400" }),
  );
  expect(result.current.validate()).toBe(null);
  await act(async () => {
    await result.current.save();
  });
  expect(saveServerIntegrationChanges).toHaveBeenCalledOnce();
  expect(
    vi
      .mocked(saveServerIntegrationChanges)
      .mock.calls[0]![1].upserts.map((item) => [
        item.id,
        item.settings.channel_id,
      ]),
  ).toEqual([
    ["existing", "402"],
    ["second", "400"],
  ]);
});

test("disabling an existing card remains valid after publishing permission is revoked", async () => {
  vi.mocked(listServerDiscord).mockResolvedValue({
    ...overview,
    items: [{ ...original, card: { enabled: true, fields: [] } }],
  });
  vi.mocked(serverDiscordChannels).mockResolvedValue([
    { id: "400", name: "public", available: false, missing: ["Send Messages"] },
  ]);
  const { result } = renderHook(() => useServerIntegrations(server, true));
  await waitFor(() => expect(result.current.channels.discord).toHaveLength(1));
  const item = result.current.items[0]!;
  act(() =>
    result.current.update(item.key, {
      ...item.value,
      card: { ...item.value.card, enabled: false },
    }),
  );
  expect(result.current.validate()).toBe(null);
  await act(async () => {
    await result.current.save();
  });
  expect(saveServerIntegrationChanges).toHaveBeenCalledOnce();
});

test("shared wizard discovers reported card fields and retains them through snapshot expiry", async () => {
  const snapshot: MonitoringSnapshot = {
    collected_at: 1,
    incomplete: false,
    sample_interval_seconds: 1,
    servers: [
      {
        id: server.id,
        name: "Server",
        spark_id: "spark",
        spark_name: "Spark",
        cpu_limit: 1,
        memory_mb: 1024,
        disk_mb: 1024,
        received_at: 1,
        usage: {
          server_id: server.id,
          state: "running",
          cpu_percent: 0,
          memory_bytes: 0,
          disk_bytes: 0,
          sampled_at: 1,
          disk_sampled_at: 1,
          sample_age_seconds: 0,
          disk_age_seconds: 0,
          container_error: "",
          disk_error: "",
          disk_quota_enforced: false,
          disk_quota_error: "",
        },
        game: {
          updated_at: 1,
          fresh_for_seconds: 60,
          status: "fresh",
          state: {
            players: 0,
            capacity: 20,
            fields: { invite_code: "test", pvp: false },
            field_labels: { invite_code: "Join code" },
          },
        },
      },
    ],
  };
  vi.mocked(readMonitoring)
    .mockResolvedValueOnce(snapshot)
    .mockResolvedValue({ ...snapshot, servers: [] });
  const { result } = renderHook(() =>
    useServerIntegrations({ ...server, game_integration: true }, true),
  );
  const expected = {
    players: "players",
    capacity: "capacity",
    "field.invite_code": "Join code",
    "field.pvp": "pvp",
  };
  await waitFor(() => expect(result.current.availableFields).toEqual(expected));
  act(() => result.current.add());
  act(() => result.current.setEditing(null));
  await waitFor(() => expect(readMonitoring).toHaveBeenCalledTimes(2), {
    timeout: 2000,
  });
  expect(result.current.availableFields).toEqual(expected);
});

test("inactive integration configuration does not start field discovery", async () => {
  renderHook(() =>
    useServerIntegrations({ ...server, game_integration: true }, false),
  );
  expect(readMonitoring).not.toHaveBeenCalled();
});

test("wizard rejects excessive card fields before calling the save API", async () => {
  const { result } = renderHook(() => useServerIntegrations(server, true));
  await waitFor(() => expect(result.current.channels.discord).toHaveLength(2));
  const item = result.current.items[0]!;
  const fields = Array.from(
    { length: IntegrationLimits.CardFields + 1 },
    (_, index) => `field.key${index}`,
  );
  act(() =>
    result.current.update(item.key, {
      ...item.value,
      card: { enabled: true, fields },
    }),
  );
  expect(result.current.validate()).toBe(
    "Select no more than 32 additional server-card fields.",
  );
  await expect(result.current.save()).rejects.toThrow("Select no more than 32");
  expect(saveServerIntegrationChanges).not.toHaveBeenCalled();
  act(() =>
    result.current.update(item.key, {
      ...item.value,
      card: {
        enabled: true,
        fields: fields.slice(0, IntegrationLimits.CardFields),
      },
    }),
  );
  expect(result.current.validate()).toBe(null);
});
