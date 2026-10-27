---
title: Notifications
description: Configure monitoring alerts, your inbox, and optional Apprise delivery.
---

**Notifications** holds messages for your account. Reading a message or marking
all messages read affects only your inbox. Server messages follow your access to
that server; Spark alerts are reserved for administrators.

## Monitoring alerts

Campfire monitors servers in the background, including when no browser is open.
Resource thresholds use each server's allocation. For example, 180% CPU usage
on a two-core allocation is 90% of that allocation.

Alerts are evaluated against the latest collected resource data. Resource
collection runs separately, every 10 seconds by default, controlled by
`CAMPFIRE_RESOURCE_INTERVAL_SECONDS`. The alert interval below controls how often
Campfire evaluates alert conditions.

| Setting                           | Default | Purpose                                          |
| --------------------------------- | ------- | ------------------------------------------------ |
| `CAMPFIRE_ALERT_CPU_PERCENT`      | `90`    | CPU threshold; `0` disables it                   |
| `CAMPFIRE_ALERT_MEMORY_PERCENT`   | `90`    | Memory threshold; `0` disables it                |
| `CAMPFIRE_ALERT_DISK_PERCENT`     | `90`    | Game-data disk threshold; `0` disables it        |
| `CAMPFIRE_ALERT_DURATION_SECONDS` | `120`   | Time an alert or recovery condition must persist |
| `CAMPFIRE_ALERT_INTERVAL_SECONDS` | `30`    | Time between alert evaluations                   |
| `CAMPFIRE_ALERT_HEALTH_ENABLED`   | `true`  | Spark connection and server health alerts        |

Health alerts cover unavailable monitoring, failed runtime states, and relevant
Spark or quota failures. They do not probe game ports. Intentionally stopped
games do not produce downtime alerts.

## External delivery

In-app notifications work without an external service. To also send monitoring
messages through Apprise, configure the panel with a complete notify endpoint:

```dotenv
CAMPFIRE_APPRISE_URL=http://apprise:8000/notify/campfire
# Optional routing tags and inline destinations:
# CAMPFIRE_APPRISE_TAGS=admins,homelab
# CAMPFIRE_APPRISE_URLS=discord://webhook_id/webhook_token
```

This is a global operator destination for monitoring messages, not per-user
routing. Treat URLs containing credentials as secrets. Restart or recreate the
panel after changing its environment settings.

Administrators can test delivery and inspect or retry failed deliveries from
Notifications. External delivery failures leave the in-app message intact.
