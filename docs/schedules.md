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

Inspect schedule history for the run's status and any reported error. **Run now**
lets you trigger a schedule explicitly. It still uses the same action requirements
and permissions as a timed execution.

Backup creation requires a stopped game. Arrange the stop and later start
separately rather than expecting a backup action to stop the server itself.
See [backups](/Campfire/server-backups/) for restoration and off-host storage.
