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

Alerts are evaluated after each resource collection cycle, every 10 seconds by
default. A threshold or recovery condition must persist for 120 seconds by default
before a notification is created. Configure collection frequency, thresholds,
and duration in [Monitoring and alerts](/Campfire/configuration/#monitoring-and-alerts).
Set an individual resource threshold to `0` to disable it.

Health alerts cover unavailable monitoring, failed runtime states, and relevant
Spark or quota failures. They do not probe game ports. Intentionally stopped
games do not produce downtime alerts.

## External delivery

### Apprise monitoring messages

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

### Discord game-server messages

Discord destinations send selected game-server events and can publish live server
cards. They are configured separately from Apprise and personal inboxes. See
[Discord integration](/Campfire/discord/) for bot setup, destinations, filters,
cards, and delivery recovery.

## Activity and delivery troubleshooting

Open a server's **Activity** tab to investigate what happened without changing
anyone's personal inbox read state:

1. Choose **Operations** to follow operations and copy or transfer jobs, **Game
   events** for mod publications, or **Notifications** for delivery outcomes.
2. Use the available filters to narrow the history. Game events offer event-type
   and field filters; notifications can be filtered by provider and destination.
3. Open a source entry to inspect its details and related notifications. Open a
   notification to inspect its destination, attempts, and source, or follow its
   Discord link when available.
4. For a known failed delivery, fix the connection or permissions and choose
   **Retry**. For an unconfirmed delivery, use **Check delivery**: the original
   send may have succeeded. An unresolved check can remain unconfirmed; see
   [Discord delivery recovery](/Campfire/discord/#delivery-and-card-recovery).

Access follows the source's permissions; retry actions require the corresponding
configuration or notification-management permissions. See
[Accounts and access](/Campfire/authentication/#permissions-for-related-features).

Job retry and cancellation act on the job itself, not its notifications. Follow
[copy and transfer recovery](/Campfire/server-transfers/) for those operations.
Administrators can use **Jobs** for a cross-server job view.
