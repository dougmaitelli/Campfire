---
title: Schedules, jobs, and audit API
description: Schedule server actions, inspect their executions, and track copy and transfer jobs.
---

Paths are relative to `/api/v1`. Schedules describe future work, executions record individual attempts, and jobs track the steps of a server copy or transfer. Keep their IDs separate when polling or retrying.

## Schedules and executions

| Method and path                            | Scope                                                                    | Request and result                                                                                                         |
| ------------------------------------------ | ------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------- |
| `GET /schedules`                           | `server.schedules.read` per server                                       | Array of visible schedules. Optional `server_id` filter.                                                                   |
| `POST /schedules/preview`                  | Authenticated                                                            | Send `cron_expression` and `time_zone`. Returns the next three Unix timestamps as `next_runs` without creating a schedule. |
| `POST /schedules`                          | `server.schedules.manage` and action permission                          | Create a schedule using the fields below. Returns `201` with the saved schedule and its `id`.                              |
| `PUT /schedules/{scheduleID}`              | Same as create                                                           | Send the schedule configuration for the same `server_id`. Returns the updated schedule.                                    |
| `PATCH /schedules/{scheduleID}`            | Same as create                                                           | Send `{"enabled":false}` or `{"enabled":true}`. Returns the updated schedule.                                              |
| `DELETE /schedules/{scheduleID}`           | Same as create                                                           | Remove the schedule. Returns `204`.                                                                                        |
| `POST /schedules/{scheduleID}/run`         | `server.schedules.run` and action permission                             | Trigger an immediate run. Returns `202` with an execution record.                                                          |
| `GET /schedules/executions`                | `server.schedules.read` per server or authorized platform access         | Page of visible executions. Filter by `server_id` or `schedule_id`.                                                        |
| `POST /schedules/executions/{runID}/retry` | Action permission, plus `server.schedules.run` for a scheduled execution | Create another attempt for an eligible failed or interrupted execution. Returns `202` with the new run.                    |

For example, this `POST /schedules` body creates a **disabled** daily restart. Substitute an actual server ID. To inspect the timing first, send just `cron_expression` and `time_zone` to the preview endpoint.

```json
{
  "server_id": "example-survival",
  "name": "Daily restart",
  "action": "restart",
  "enabled": false,
  "only_when_running": true,
  "cron_expression": "0 4 * * *",
  "time_zone": "UTC"
}
```

Use a five-field cron expression for recurrence, or omit it and supply `next_run_at` in Unix seconds for a one-time execution. Actions are `start`, `stop`, `restart`, `kill`, `backup`, and `command`. A `command` schedule needs the `command` text and `server.console.write`. A backup schedule accepts `backup_mode` (`full` or `savedata`) and `backup_stop_server`. Scheduled work retains the author's token scopes and rechecks account access at dispatch. See [schedule syntax and execution behavior](/Campfire/schedules/).

Use the returned run `id` to identify the attempt in execution history. Inspect `state`, `message`, `started_at`, and `finished_at`. Terminal states include `succeeded`, `failed`, `interrupted`, `cancelled`, and `skipped`.

## Copies, transfers, and jobs

| Method and path                 | Access                                     | Request and result                                                                                                                                                                                                                                                       |
| ------------------------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `GET /servers/{id}/copy`        | `server.configure` and `servers.create`    | Returns a copy template with `server`, `ember`, and backup `policy`.                                                                                                                                                                                                     |
| `POST /servers/{id}/copy`       | Same as template, plus target Spark access | Send the new [server configuration](/Campfire/api/servers/#configuration-and-lifecycle), `copy_files`, and optionally `owner_user_id` and `policy`. Copying files also requires `server.files.read` on the source. Returns `201` with the new server and its `Location`. |
| `POST /servers/{id}/quick-copy` | Same as copying with files                 | Send `{"spark_id":"…"}` to copy with automatically chosen settings. Returns `201` with the new server.                                                                                                                                                                   |
| `POST /servers/{id}/transfers`  | `sparks.manage` and `servers.manage_all`   | Send `target_spark_id` and the target `ports` array. Returns `202` with job progress.                                                                                                                                                                                    |
| `GET /servers/{id}/jobs`        | `server.view` with job-type visibility     | Page of visible jobs involving this server.                                                                                                                                                                                                                              |
| `GET /jobs`                     | Administrator with `*`                     | Page of platform jobs. Optional `type` (`copy`, `transfer`) and `status` filters.                                                                                                                                                                                        |
| `POST /jobs/{jobID}/retry`      | Job has `can_retry` for caller             | Send `{}`. Returns `202` with the updated job.                                                                                                                                                                                                                           |
| `POST /jobs/{jobID}/cancel`     | Job has `can_cancel` for caller            | Send `{}`. Returns `202` with the updated job.                                                                                                                                                                                                                           |

After a copy, follow the new server's `/jobs` endpoint to track file work. After a transfer, follow the returned job `id`. Possible job statuses are `pending`, `running`, `failed`, `cancelling`, `completed`, and `cancelled`. Use `waiting_for_sparks` to distinguish a job waiting on a host from one making progress. Refresh `can_retry` and `can_cancel` before exposing controls because both access and job step affect the available actions. See [copies and transfers](/Campfire/server-transfers/) for recovery behavior.

## Audit

| Method and path | Scope        | Response or query                                                                             |
| --------------- | ------------ | --------------------------------------------------------------------------------------------- |
| `GET /audit`    | `audit.view` | Newest-first page of audit events. Optional `action` and `target` filters match exact values. |

Each event has a stable `id`, `username`, `action`, `target`, `outcome`, HTTP `status`, and Unix-second `created_at`. `api_token_id` identifies the personal token when applicable. To retrieve one server's records, filter by `target=server:<id>`. See [Read audit history by page](/Campfire/api/guide/#read-audit-history-by-page) for the request and pagination behavior.
