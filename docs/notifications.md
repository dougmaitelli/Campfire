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
default, controlled by `CAMPFIRE_RESOURCE_INTERVAL_SECONDS`. Set
`CAMPFIRE_ALERT_DURATION_SECONDS` to control how long an alert or recovery
condition must persist before a notification is created.

| Setting                           | Default | Purpose                                          |
| --------------------------------- | ------- | ------------------------------------------------ |
| `CAMPFIRE_ALERT_CPU_PERCENT`      | `90`    | CPU threshold; `0` disables it                   |
| `CAMPFIRE_ALERT_MEMORY_PERCENT`   | `90`    | Memory threshold; `0` disables it                |
| `CAMPFIRE_ALERT_DISK_PERCENT`     | `90`    | Game-data disk threshold; `0` disables it        |
| `CAMPFIRE_ALERT_DURATION_SECONDS` | `120`   | Time an alert or recovery condition must persist |
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

## Discord bot setup

For a setup walkthrough, token rotation, backup instructions, and recovery
actions, see [Discord integration](../discord/).

Administrators can open **Integrations** from the account menu to configure one
Discord bot connection and Discord server. Create an application in the
[Discord developer portal](https://discord.com/developers/applications), add its
bot to your Discord server, choose **Add integration → Discord**, and enter its
bot token. Available servers load automatically; use **Add bot to Discord** beside
the server selector to install it with the required permissions. Returning to the
panel refreshes the server choices. Choose one and save.

The bot needs **View Channel**, **Send Messages**, **Embed Links**, **Attach Files**, and **Read
Message History** in the destination channel. Campfire checks role permissions
and channel overrides, including member-specific overrides. The channel selector
explains missing permissions before allowing a test message.

Click a saved connection row to change its Discord server or rotate its token with
**Change bot token**. Available servers load automatically using the saved token.
Expand **Test delivery** to load channels and send a test message. The connection
list provides an enable toggle; disabling remains possible if Discord is
unavailable or the token has been revoked. **Remove integration** disconnects its
game servers and removes its saved token and delivery records. Existing messages
remain in Discord.

Tokens are encrypted with AES-256-GCM in the panel's `integration-secrets` state
directory. Credential files and the encryption key are owner-readable/writable
only. The database stores an opaque reference, and API responses and audit events
exclude the token. Back up this directory together with the panel database; a
database backup alone does not contain the credentials or their encryption key.

Open a game's console page and choose **Actions → Edit server → Integrations** to configure
that server's destinations. Add one for each channel, with independent
notification categories, filters, and optional server card fields. For example,
publish backup events to an admin channel and the card to a public channel.
Click a destination row to edit it. Channels load when the edit wizard opens.
Changes remain drafts until the wizard's final **Save changes**; cancelling the
wizard discards them.
Game-state field choices come from values reported by the game;
saved choices remain available while the server is offline. Settings take effect
without restarting the game, and disabling the server integration keeps its
channel and preferences for later reuse.

Managing these settings requires the existing `server.configure` permission.
Owners, administrators, and server editors can configure integrations.
Sharing console alerts also requires console-read permission, and backup events
require backup-read permission. Global bot setup remains administrator-only.
Notification delivery runs automatically for enabled subscriptions. Enabling the
server card publishes one live card independently of notification subscriptions.

The card includes the server name, observed runtime status, and game address with
host ports when available. Wildcard bind addresses are not shown as connectable
addresses. Select player count, player capacity, and individual reported game
fields to include. For example, a mod can report an invite code or a string of
online player names. Available choices come from reported values, with saved
choices retained while offline. Game fields use their reported keys as labels.
Unselected fields, server variables, and console contents
are excluded. No structured player-roster protocol is added.

The preview uses unsaved field choices without publishing those choices or
contacting Spark. Save changes to update the live card. Selected values become
**Unavailable** when the game snapshot
or runtime observations expire, the Spark disconnects, the server stops or moves,
or a lifecycle operation invalidates its game state. Saved field choices remain
available while offline. Discord rendering escapes user text, suppresses mentions,
and bounds embed text; large selections continue in a second embed in the same
message.

Campfire edits the saved card when its content changes, coalesces rapid updates,
and periodically checks quiet cards. A confirmed deleted message is recreated.
Permission failures, missing channels, and network errors keep the saved message
identity and do not trigger replacement cards. Channel moves wait for deletion
of the old card before creating the new one. Disabling the server card, binding,
or global connection requests removal of the existing card; an unavailable bot
can leave cleanup pending until access is restored.

**Card publishing** tracks the saved card's published, pending, failed, or
unconfirmed state. Disabled cards do not show a status row; newly enabled cards
prompt you to save before publishing. Use **Retry card** after fixing a known failure. **Check card**
searches for a card whose creation response was lost and never blindly posts a
replacement. The creation identity and external message reference survive panel
restarts. If bounded history cannot confirm an ambiguous creation, publishing
pauses visibly to avoid duplicates, including during a channel change.

Game-event subscriptions can select one or more event types, each with its own
optional field conditions. With no rules, all types are included. Event-type rows
are alternatives; conditions inside a row must all match. A condition on
`player.joined` does not filter another row such as `game.announcement`, and an
event missing a required field does not match that row. Field comparisons use exact,
case-insensitive values; **Any value** checks that the field exists. Dropdowns use
the same retained event history as the Activity tab, and saved choices remain
available after their source events expire. No field condition is added until
you choose **Add field filter**.

The power category records terminal operation results and fresh observed state
changes, including external stops and restarts. Accepting a command does not
report success, and interrupted results remain explicitly interrupted. Backup
events include backup and restore operation outcomes. Console alerts contain
the alert's guidance without raw console lines or command contents.

Only new events matching enabled subscriptions are recorded for delivery;
enabling an integration does not replay existing activity. Recording is atomic
with each source update, and repeated source observations do not duplicate its
messages.

Notification history is not currently displayed in the panel. The per-binding
messages API reports pending, delivered, failed, and unconfirmed messages. Transient
failures use bounded retries; Discord rate limits defer delivery until the
requested cooldown expires. One unavailable destination does not stop other
destinations. Disabling a connection or server integration stops new sends; a
request already in flight may still complete.

After fixing a failed delivery's connection or permissions, the message retry API
can retry a known failure. For unconfirmed sends it searches channel history for the
existing message. Discord's nonce checking suppresses immediate duplicate sends,
and each notification includes a stable footer reference for recovery. If a lost
response cannot be reconciled, automatic resending pauses to avoid
posting a duplicate. Receipt recovery never posts a replacement message.

Bot messages suppress automatic mentions and bound the embedded text to
Discord's limits. Credentials, raw provider response bodies, raw console lines,
and console commands are excluded from delivery status and failure messages.
