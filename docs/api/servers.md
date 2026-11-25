---
title: Servers and monitoring API
description: Look up servers, manage their configuration and lifecycle, and consume live and historical readings.
---

Paths are relative to `/api/v1`. Use the `id` from the server inventory wherever a route contains `{id}`. The **scope** column names the permission checked on the target. See [authentication and prerequisites](/Campfire/api/#choose-scopes).

## Inventory and measurements

| Method and path                     | Scope                         | Response or query                                                                                                           |
| ----------------------------------- | ----------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `GET /servers`                      | `server.view` for each result | Array of visible servers, including `id`, `name`, `spark_id`, `ember_id`, cached `status`, and effective `permissions`.     |
| `GET /servers/{id}`                 | `server.view`                 | One server with its configuration, reservations, permission flags, and Ember and Spark information.                         |
| `GET /servers/status`               | `server.view` for each result | Array of runtime status objects keyed by their `server_id` field.                                                           |
| `GET /servers/{id}/status`          | `server.view`                 | Current runtime status including `state`, `ready`, `busy`, and available operation details.                                 |
| `GET /monitoring`                   | Filtered by current grants    | Fleet snapshot with `collected_at`, `sample_interval_seconds`, `servers`, `incomplete`, and Spark readings when authorized. |
| `GET /servers/{id}/metrics`         | `server.view`                 | Latest stored resource usage, including timestamps and available CPU, memory, and disk measurements.                        |
| `GET /servers/{id}/metrics/history` | `server.view`                 | CPU and memory series. Accepts `from` and `to` in Unix seconds and `limit` in points.                                       |

History defaults to the last 24 hours and up to 240 points. Request up to 1,000 points across at most 365 days. The result has `from`, `to`, `interval_seconds`, and `samples`. Each sample includes `collected_at`, counts of measurements, and `usage.cpu_percent` and `usage.memory_bytes`, which can be null.

## Configuration and lifecycle

| Method and path                    | Scope                                       | Request and result                                                                                                                   |
| ---------------------------------- | ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `POST /servers`                    | `servers.create` and access to target Spark | Send a server configuration, optionally `owner_user_id` or a prepared `backup_token`. Returns `201`, the server, and its `Location`. |
| `GET /servers/{id}/editor`         | `server.configure`                          | Returns Spark and Ember choices, S3 availability, and the proposed Ember configuration when available.                               |
| `PUT /servers/{id}`                | `server.configure`                          | Send the full editable server configuration. Returns the updated server.                                                             |
| `POST /servers/{id}/configuration` | `server.operate`                            | Send `{}` to deliver saved configuration to Spark. Returns a deployment receipt.                                                     |
| `POST /servers/{id}/ember`         | `servers.manage_all`                        | Apply a reviewed server configuration with `expected_ember_id` and `expected_ember_revision`. Returns the server.                    |
| `POST /servers/{id}/actions`       | Depends on action below                     | Send an action object. Returns `202` with runtime status.                                                                            |
| `DELETE /servers/{id}`             | `server.delete`                             | Delete the server and its game data. Returns `204`.                                                                                  |

For creation, get available Sparks and Embers from [provisioning](/Campfire/api/administration/#provisioning-and-access). Configuration fields are top-level JSON properties. They include `name`, `description`, `spark_id`, `ember_id`, `image`, `game_address`, `game_integration`, `variables`, `cpu_limit`, `memory_mb`, `disk_mb`, `ports`, and startup settings. A port entry has `bind_ip`, `host_port`, `container_port`, and `protocol`. Use the chosen Ember's image and variable definitions and the target Spark's port allocations. Read-only response properties such as `id` and `permissions` do not belong in the configuration payload.

| Value of `action`                             | Required server permission | Additional request fields                                                                                |
| --------------------------------------------- | -------------------------- | -------------------------------------------------------------------------------------------------------- |
| `install`, `start`, `stop`, `restart`, `kill` | `server.operate`           | Send the action alone, for example `{"action":"start"}`.                                                 |
| `update`, `reinstall`                         | `server.reinstall`         | Reinstall accepts `mode` of `keep` or `clean`. Clean reinstall removes existing game files.              |
| `backup`                                      | `server.backups.create`    | `backup_mode` accepts `full` or `savedata`. Save-data backups require an Ember that declares save paths. |
| `restore`                                     | `server.backups.restore`   | Set `backup` to the archive's `name` from the server backup list.                                        |

A runtime status can include `operation_id`, `operation`, `operation_state`, `progress`, and `error`. Use [Start a server and check the result](/Campfire/api/guide/#start-a-server-and-check-the-result) for a working request and polling guidance.

## Console

| Method and path                    | Scope                  | Request and result                                                                         |
| ---------------------------------- | ---------------------- | ------------------------------------------------------------------------------------------ |
| `GET /servers/{id}/console`        | `server.console.read`  | Runtime status with recent console output in `logs` when available.                        |
| `GET /servers/{id}/console/stream` | `server.console.read`  | Long-lived `text/event-stream` with named events and JSON in the SSE `data` field.         |
| `POST /servers/{id}/console`       | `server.console.write` | Send `{"command":"…"}` to a server that supports console commands. Returns runtime status. |

Stream events are `connecting`, `ready`, `output` (`content`), `heartbeat`, `ended`, and `denied`. Heartbeats are sent every five seconds. Stop on `denied`. On an interrupted connection, back off before reconnecting. Read the recent snapshot if needed. The stream does not supply replay IDs. See the [console request and sample event](/Campfire/api/guide/#follow-live-console-output).

## Events and activity

| Method and path                                       | Scope                                         | Response or query                                                                                                                                 |
| ----------------------------------------------------- | --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /servers/{id}/events`                            | `server.view`                                 | Page of retained game events. Filter by `type`, `search`, Unix-second `from` and `to`, or `data.<field>`.                                         |
| `GET /servers/{id}/events/filters`                    | `server.view`                                 | Available event `types`, `fields`, and optional `field_labels`.                                                                                   |
| `GET /servers/{id}/events/{event}/media/{attachment}` | `server.view`                                 | Raw media with its recorded content type. Use the numeric event ID and attachment ID. Add `download=1` for attachment disposition.                |
| `GET /servers/{id}/log-alerts`                        | `server.view`                                 | Array of detected alerts. The visible detail depends on console access.                                                                           |
| `POST /servers/{id}/log-alerts/{alert}/dismiss`       | `server.operate`                              | Send `{}` to dismiss an alert. Returns `204`.                                                                                                     |
| `GET /servers/{id}/activity`                          | `server.view`                                 | Page of visible operations, jobs, events, and notifications. Filters include `kind`, `state`, `search`, `type`, `from`, `to`, and `data.<field>`. |
| `GET /servers/{id}/activity/filters`                  | `server.view`                                 | Return filter choices for the supplied `kind`.                                                                                                    |
| `GET /servers/{id}/activity/{entryKey}`               | `server.view`                                 | Read one activity entry and available detail, including job progress where applicable.                                                            |
| `GET /servers/{id}/activity/{entryKey}/notifications` | `server.view`                                 | Page of visible delivery records associated with an entry.                                                                                        |
| `POST /servers/{id}/activity/{entryKey}/retry`        | Eligible delivery and permission to manage it | Send `{}` to retry the selected notification delivery.                                                                                            |

Use the `key` returned by the activity list for `{entryKey}`. Visibility also follows the entry's source, such as schedule, backup, or console permissions. For copies, transfers, and per-server jobs, see the [automation reference](/Campfire/api/automation/#copies-transfers-and-jobs).
