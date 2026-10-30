---
title: Schedules
description: Automate recurring server actions and inspect their execution history.
---

Schedules are actions you create to run later. Use **Schedules** to configure
recurring work and inspect previous scheduled executions. Starting a server or
creating a backup directly from its controls does not create a schedule.

## Create a schedule

1. Choose the server and action you want to automate.
2. Choose a one-time execution or set a recurring cron expression and time zone,
   and complete the action-specific options.
3. Enable the schedule and save it. Review the next execution time.

Use a named time zone that matches your intended local time. Account for daylight
saving changes when choosing a time, or use UTC for a fixed reference.

The schedule runs with its owner's current access. Keep the account enabled and
the required server permissions in place. The panel must be running to dispatch
scheduled work, and the Spark must be available to execute it.

## Check the result

Open a schedule from its row to view its settings and execution history. The
drawer shows only that schedule's runs and can load older attempts.
`server.schedules.read` allows inspection and requires `server.operate`. Editing requires
`server.schedules.manage` and the permission for the scheduled action, such as
`server.operate` for a restart or `server.backups.create` for a backup.

Inspect schedule history for the run's status and any reported error. **Run now**
lets you trigger a schedule explicitly with `server.schedules.run` and the
underlying action's permission. The same grants are required to retry a scheduled
execution. Jobs created or enabled through an API token retain its scope limit
and recheck the account's current access before dispatch.

Backup creation requires a stopped game. Arrange the stop and later start
separately rather than expecting a backup action to stop the server itself.
See [backups](/Campfire/server-backups/) for restoration and off-host storage.
