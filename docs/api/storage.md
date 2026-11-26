---
title: Files and backups API
description: Browse game files, safely update content, transfer binary data, and work with Campfire backups.
---

Paths are relative to `/api/v1`. Game file paths are rooted at the server's data directory. For example, `/server.properties` refers to a file in that directory. Use `/` for the root. Directory paths in responses end with a slash.

## Files

| Method and path                    | Required server scope  | Request and result                                                                                                                                                |
| ---------------------------------- | ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /servers/{id}/files`          | `server.files.read`    | Optional `path` query parameter. Returns `server_id`, `path`, and `entries` when present. Entries include `name`, `kind`, byte `size`, `mode`, and `modified_at`. |
| `GET /servers/{id}/files/download` | `server.files.read`    | Set `path` to a file. Returns raw bytes as an attachment.                                                                                                         |
| `GET /servers/{id}/files/limits`   | `server.files.write`   | Returns `max_upload_bytes` for this deployment.                                                                                                                   |
| `POST /servers/{id}/files`         | Depends on `operation` | Send a file operation object. Returns a file result such as a listing, base64 `data`, a new `revision`, or an `upload_id`.                                        |

For example, `GET /api/v1/servers/example-survival/files/download?path=%2Fserver.properties` returns the contents of `/server.properties` as `application/octet-stream`. The `Content-Disposition` header supplies the download filename.

### File operations

| Scope                 | Values of `operation`                                                                                                          | Inputs                                                                                                                            |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------- |
| `server.files.read`   | `list`, `stat`, `read`, `read.chunk`                                                                                           | Supply `path`. `read` returns base64 `data` and a content `revision`. Chunk reads accept a byte `offset`.                         |
| `server.files.write`  | `write`, `mkdir`, `rename`, `chmod`                                                                                            | `write` takes `path`, base64 `data`, and `revision`. For a rename supply `destination`. For chmod send an integer `mode`.         |
| `server.files.write`  | `upload.begin`, `upload.chunk`, `upload.commit`, `upload.cancel`                                                               | Stage a file at `path` using the chunk sequence below.                                                                            |
| `server.files.write`  | `compress`, `extract`                                                                                                          | Compress selected `paths` into an archive or extract an archive at `path` to `destination`.                                       |
| `server.files.write`  | `compress.start`, `compress.status`, `compress.cancel`, `extract.start`, `extract.status`, `extract.resolve`, `extract.cancel` | Track a long-running archive operation using its `compression` or `extraction` object. Resolution answers an extraction conflict. |
| `server.files.delete` | `delete`                                                                                                                       | Delete the file or directory at `path`.                                                                                           |

For a small edit, first send `{"operation":"read","path":"/settings.txt"}`. Include the returned revision when you write the new contents. For a new file use `"revision":"absent"`. A `409` signals a conflict or busy resource. Re-read before deciding whether to overwrite a concurrent edit.

For a chunked upload, post these operations to the same files endpoint in order:

1. Send `upload.begin` with `path`, byte `size`, and `revision`. Use the revision from `stat` for an existing upload destination or `"absent"` for a new one. Save the returned `upload_id`.
2. Send `upload.chunk` with the same `path`, `upload_id`, `offset`, and base64 `data`. Start at offset zero and advance by the decoded byte count. Limit each chunk to 1 MiB before encoding.
3. Send `upload.commit` with `path` and `upload_id` to publish the complete file. Use `upload.cancel` to abandon staging. If the commit response is lost, inspect the destination before starting over.

## Backups and retention

A library entry has an `id`, a server-side archive `name`, and an editable `display_name`. Use `id` in library URLs and `name` in per-server URLs and restore requests. To create or restore a backup, use [server actions](/Campfire/api/servers/#configuration-and-lifecycle).

| Method and path                       | Required scope                        | Request and result                                                                                                                         |
| ------------------------------------- | ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `GET /servers/{id}/backups`           | `server.backups.read`                 | Lists server `entries`, including archive names, sizes, and available storage metadata.                                                    |
| `GET /servers/{id}/backups/{name}`    | `server.backups.download`             | Streams an `application/gzip` archive.                                                                                                     |
| `PATCH /servers/{id}/backups/{name}`  | `server.backups.create`               | Send `display_name`, `pinned`, or both. Returns updated metadata.                                                                          |
| `DELETE /servers/{id}/backups/{name}` | `server.backups.delete`               | Queues deletion. Returns `202` with `{"state":"deleting"}`.                                                                                |
| `GET /servers/{id}/backup-policy`     | `server.configure`                    | Returns `policy` and `s3_available`.                                                                                                       |
| `PUT /servers/{id}/backup-policy`     | `server.configure`                    | Send the policy object itself, including `storage`, local and remote retention limits, and `savedata` retention. Returns the saved policy. |
| `GET /backups`                        | Filtered by archive access            | Array of visible library archives. Each has `can_download`, `can_edit`, `can_delete`, and `can_create_server`.                             |
| `GET /backups/{backupID}`             | `server.backups.download` on archive  | Streams the library archive.                                                                                                               |
| `PATCH /backups/{backupID}`           | `server.backups.create` on archive    | Send `display_name` or `pinned`. Returns updated metadata.                                                                                 |
| `DELETE /backups/{backupID}`          | `server.backups.delete` on archive    | Queues deletion. Returns `202`.                                                                                                            |
| `POST /backups/{backupID}/prepare`    | Archive download and `servers.create` | Prepare an existing library backup for server creation. Returns an import preview with a `token`.                                          |

Linked archives use the original server's access rules. Independent library archives use archive ownership and account permissions. See [backup modes and storage policy](/Campfire/server-backups/) for the meaning of each retention setting.

## Import a portable archive

Use the same authenticated account throughout the upload. The short-lived import `token` identifies this upload in the URL. Continue sending the personal API token in the authorization header.

| Step                       | Method and path                             | Request and result                                                                                                                                                                                                                                                  |
| -------------------------- | ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Begin                      | `POST /backup-imports`                      | Send byte `size` and optional archive `name`. Set `server_id` for an existing server (`server.backups.create`). Omit it for a library upload (`servers.create`). For storage only, set `store_only: true` and omit `server_id`. Returns `201` with `{"token":"…"}`. |
| Upload                     | `POST /backup-imports/{token}/chunks`       | Send `{"offset":0,"data":"…"}` for each consecutive chunk, at most 1 MiB of decoded bytes. Returns `200` and `{}`.                                                                                                                                                  |
| Inspect                    | `POST /backup-imports/{token}/inspect`      | Inspect the complete archive. Returns `token`, `mode`, `configuration`, `ember`, `import_ember`, and `backup_id` when available. A library upload is stored at this step.                                                                                           |
| Publish to existing server | `POST /servers/{id}/backup-imports/{token}` | Requires `server.backups.create`. Returns `201` with the resulting archive `name`. Restoring it is a separate server action.                                                                                                                                        |
| Abandon                    | `DELETE /backup-imports/{token}`            | Release the temporary import.                                                                                                                                                                                                                                       |

To create a server, use the inspected configuration as a starting point, choose placement and ports, and send the import token as `backup_token` to `POST /servers`. The [server configuration reference](/Campfire/api/servers/#configuration-and-lifecycle) describes the payload.
