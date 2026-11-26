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

Console-command schedules also require the Ember to support
[console commands](/Campfire/ember-schema/#interactive-console-commands). Unsupported
command schedules cannot be saved or enabled. If support is disabled after a
schedule is created, its next attempted execution fails with **This game does not
support console commands.** No command is sent to Spark.

### Cron syntax

Recurring schedules use five fields, with no seconds field:

```text
minute  hour  day-of-month  month  day-of-week
0–59    0–23  1–31          1–12   0–6 (Sunday = 0)
```

Use `*` for every value, commas for lists, hyphens for ranges, and `/` for steps.
Month and weekday names such as `JAN` and `MON` are also accepted. Choose the time
zone separately; do not put it inside the expression. Shortcuts such as `@daily`
and six-field expressions are not supported.

| Expression         | Runs                                         |
| ------------------ | -------------------------------------------- |
| `0 4 * * *`        | Every day at 04:00 in the selected time zone |
| `*/30 * * * *`     | Every hour at minutes 00 and 30              |
| `0 18 * * MON-FRI` | Weekdays at 18:00 in the selected time zone  |

When both day-of-month and day-of-week are restricted, either can match. Use `*`
for the day field you do not intend to restrict. Review the displayed next run
after saving, especially for times affected by daylight saving changes.

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

For backup schedules, choose **Full server** or **Save data only**, and optionally
**Stop server during backup**. See [backup types](/Campfire/server-backups/#choose-a-backup-type)
and [scheduled backup behavior](/Campfire/server-backups/#scheduled-backups)
for save-data requirements, consistency, and stop/restart handling. If an Ember's
save-data declarations are removed, a save-data schedule fails rather than
silently switching to a full backup.
