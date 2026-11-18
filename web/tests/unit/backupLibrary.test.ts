import { expect, test, vi } from "vitest";
import {
  listAllBackups,
  listBackups,
  updateBackup,
  deleteArchive,
  archiveURL,
} from "../../src/api/backups";

const archive = {
  id: "archive-one",
  name: "full-snapshot.tar.gz",
  display_name: "Uploaded world",
  pinned: false,
  portable: true,
  mode: "full",
  size: 123,
  modified_at: "2026-11-17T12:00:00Z",
  local: true,
  remote: false,
  state: "available",
  server_name: "",
  can_download: true,
  can_edit: true,
  can_delete: true,
  can_create_server: true,
};

test.each([undefined, null, "server-one"])(
  "backup identities do not depend on association %s",
  async (serverID) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json([
        { ...archive, server_id: serverID },
        { ...archive, id: "archive-two", server_id: serverID },
      ]),
    );
    const result = await listAllBackups(new AbortController().signal);
    expect(result.map((item) => item.id)).toEqual([
      "archive-one",
      "archive-two",
    ]);
    expect(result.map((item) => item.server_id)).toEqual([
      serverID ?? "",
      serverID ?? "",
    ]);
    expect(result.every((item) => item.can_create_server)).toBe(true);
  },
);

test.each([
  { id: "" },
  { id: undefined },
  { id: 123 },
  { server_id: 123 },
  { local_spark_id: 123 },
  { local_spark_name: null },
  { can_delete: "yes" },
  { can_create_server: "yes" },
])("malformed library contracts are rejected: %j", async (change) => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    Response.json([{ ...archive, ...change }]),
  );
  await expect(listAllBackups(new AbortController().signal)).rejects.toThrow(
    "Unexpected backup catalog entry",
  );
});

test("current storage Spark is separate from an absent server association", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    Response.json([
      {
        ...archive,
        local_spark_id: "spark-one",
        local_spark_name: "Storage node",
      },
    ]),
  );
  const [stored] = await listAllBackups(new AbortController().signal);
  expect(stored).toMatchObject({
    server_id: "",
    local_spark_id: "spark-one",
    local_spark_name: "Storage node",
  });
});

test.each([undefined, "", 123, archive.id])(
  "server backup listings validate stable IDs: %s",
  async (id) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        server_id: "server-one",
        entries: [{ ...archive, id }],
        policy: {
          storage: "local",
          local_keep_last: 0,
          local_max_age_days: 0,
          remote_keep_last: 0,
          remote_max_age_days: 0,
        },
        s3_available: false,
      }),
    );
    const result = listBackups("server-one", new AbortController().signal);
    if (id === archive.id) {
      await expect(result).resolves.toMatchObject({
        entries: [{ id: archive.id }],
      });
    } else {
      await expect(result).rejects.toThrow();
    }
  },
);

test("archive metadata and deletion use stable IDs, not missing server paths", async () => {
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(Response.json(archive));
  await updateBackup(archive.id, { pinned: true });
  expect(fetch).toHaveBeenLastCalledWith(
    archiveURL(archive.id),
    expect.objectContaining({ method: "PATCH" }),
  );
  fetch.mockResolvedValue(Response.json({ state: "deleting" }));
  await deleteArchive(archive.id);
  expect(fetch).toHaveBeenLastCalledWith(
    archiveURL(archive.id),
    expect.objectContaining({ method: "DELETE" }),
  );
});
