---
title: Discord integration
description: Connect a Discord bot, publish game-server notifications and cards, and recover delivery after an outage.
---

Campfire can send game-server events to Discord and maintain a live server card.
This is separate from [personal inboxes and Apprise monitoring delivery](/Campfire/notifications/).

## Connect the bot

1. Create an application and bot in the
   [Discord developer portal](https://discord.com/developers/applications).
2. Sign in to Campfire as an administrator. Open **Integrations** from the
   account menu, choose **Add integration → Discord**, and enter the bot token.
3. Use **Add bot to Discord** beside the server selector to install the bot.
   Returning to Campfire reloads the available servers. Select one and save.
4. Edit the saved connection, expand **Test delivery**, choose a channel, and
   send a test message before configuring game-server destinations.

The bot needs **View Channel**, **Send Messages**, **Embed Links**, **Attach Files**,
and **Read Message History** in each destination text channel. Channel overrides
can remove permissions granted by a role. Campfire checks effective permissions
and explains missing access in its channel selector. The authorization link uses
Discord's `bot` scope.

Global setup is administrator-only. Per-server configuration requires
`server.configure` and its prerequisites. Sharing console alerts also requires
console-read permission; sharing backup events requires backup-read permission.

## Add server destinations

1. Open a game's console and choose
   **More server actions → Edit server → Integrations → Add integration**.
2. Select the saved integration and a channel, then enable the destination.
3. Choose notification categories, event filters, and optional live-card fields.
4. Finish the wizard with **Save changes**. **Cancel** discards unsaved changes.
5. Check matching messages in Discord and delivery outcomes in the server's
   **Activity → Notifications** tab.

Add separate destinations for different audiences, such as backup events in an
admin channel and a server card in a public channel. Each destination has its own
filters, card choices, and history. Click a destination row to edit it. Changes
take effect without restarting the game. Disabling a destination preserves its
channel and preferences for reuse.

Only new matching events are delivered; enabling a destination does not replay
existing activity. Power notifications report operation results and observed
state changes, not merely accepted commands. Backup notifications cover backup
and restore outcomes. Console alerts share guidance, not raw console lines or
command contents.

## Filter game events

Select event types and add optional field conditions to each type. With no rules,
all event types are included. Rows are alternatives; conditions within a row must
all match. For example, a condition on `player.joined` does not filter a separate
`game.announcement` row.

Field comparisons are exact and case-insensitive. **Any value** checks that the
field exists; an event missing a required field does not match. Choose **Add field
filter** to add a condition. Suggestions come from retained game-event history;
saved choices remain available after their source events expire.

## Live server cards

Enable the server card to publish a live status message, even if all notification
categories are unchecked. The card includes the server name, observed runtime
status, and the server's public connection address when available. A **Public
game address** override is used exactly as entered in the server editor. Otherwise,
the card uses the Spark's default public host and the primary mapped host port,
just like public status pages. Bind interfaces are never used as fallback hosts.

The address has reserved space and is never shortened to make room for game
fields. If it exceeds Discord's field limit after formatting, the card and preview
show a notice asking players to obtain the complete address from the server
operator. Public status pages still show the full address.

Select player count, capacity, and custom game fields to include. Choices come
from reported game state, with saved selections retained while offline. Registered
display labels are used when available. Flat lists, such as online player names,
are comma-separated; an empty list displays **None**. Unselected fields, server
variables, and console contents are excluded.

The preview shows unsaved choices without publishing them. Choose **Save changes**
to update the live card, then check **Card publishing** for its status. Selected
values become **Unavailable** when observations expire, Spark disconnects, the
server stops or moves, or a lifecycle operation invalidates game state.

Campfire updates the existing message as content changes and periodically checks
quiet cards. A confirmed deleted card is recreated. Permission failures, missing
channels, and network errors do not create replacement cards. Moving channels
requires removal of the old card first. Disabling a card, destination, or global
connection requests removal; restore bot access if cleanup remains pending.

## Game screenshots and clips

Game-event subscriptions forward image and video uploads as Discord attachments,
using the same event-type and field filters. Each completed upload follows its
event as a separate message, including files that arrive after the text message.
Upload retries do not create duplicate attachment messages.

Files are uploaded from retained game media; Discord does not need access to
protected panel URLs. The per-file allowance is 20 MiB. Larger files remain in
Activity and produce a notification explaining the limit. If retention removes a
file before delivery, the notification reports that it expired. Files already
uploaded to Discord remain there independently of panel retention.

## Delivery and card recovery

| What you see                            | What to do                                                                                           |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| No Discord servers in setup             | Check that the bot was installed in the intended Discord server, then reopen its form.               |
| An unavailable channel                  | Fix role or channel permissions, reopen the server's integration settings, and save the destination. |
| Failed notification or card             | Fix the reported issue, then choose **Retry** or **Retry card**.                                     |
| Unconfirmed notification or card        | Use **Check delivery** or **Check card** to look for the message whose creation response was lost.   |
| A deliberately deleted card             | Leave the destination enabled; Campfire periodically checks and recreates confirmed missing cards.   |
| Cleanup pending after a move or disable | Restore the bot's access to the old channel and retry.                                               |

Open **Activity → Notifications** for delivery attempts and recovery actions. See
[Activity troubleshooting](/Campfire/notifications/#activity-and-delivery-troubleshooting)
for navigation and filtering.

Transient failures retry automatically, and rate limits delay delivery. Credential
failures pause the connection; channel failures pause the affected destination.
One unavailable destination does not stop the others. Disabling a destination or
connection stops new sends, but an in-flight request may still complete.

An unconfirmed send may already exist in Discord. Checks search recent history
without posting a replacement; if no receipt can be confirmed, delivery remains
paused to avoid duplicates. Preserve saved message references during recovery.

Before deleting a game server or removing its bot from Discord, disable its card,
save, and wait for the card to disappear so cleanup can finish while access is
still available.

### Delivery history retention

Completed or failed notifications are retained for up to 30 days, capped at
10,000 per destination. Compact deduplication receipts remain for up to 90 days,
capped at 100,000 per destination. Replaying an event after its receipt expires
can send it again.

Pending work, unconfirmed sends, permission pauses, and live-card references are
retained until resolved or the integration is removed.

## Rotate a token or disconnect

Edit the global connection and choose **Change bot token** to replace a revoked
or rotated token. After saving, send a test message, check affected cards, and
retry failed notifications you still want delivered. If another administrator
changed the connection while you were editing, reload it before saving again.

The enable toggle pauses delivery without discarding settings and works even
when Discord is unavailable. **Remove integration** disconnects its game servers
and removes the saved credentials, destinations, and delivery records. Existing
Discord messages remain as history.

## Back up and restore

Preserve the panel database and the `integration-secrets` directory under the
panel's state directory as a matching backup. The directory contains the
encryption key and encrypted token files; a database backup alone is insufficient.

1. Stop the panel and take a consistent database and state backup, following
   [Upgrades and recovery](/Campfire/operations/). Include both the database and
   filesystem state when using PostgreSQL.
2. Restore the matching database, key, and credentials together. Preserve private
   ownership and permissions so only the panel's operating-system user can read
   the key and token files.
3. Start the panel, test the connection, and check publishing and delivery status.
   Pending work and saved card references survive restarts.

If the key is missing or damaged, restore it from the matching backup. Campfire
does not replace a missing key while encrypted credentials exist. Avoid clearing
message identities: an interrupted send may have succeeded before its response
was lost.

## Supported scope

- Multiple Discord connections are supported. Each connection uses one bot and
  one Discord server. Select the connection when adding a game-server destination.
  Game servers can have several destinations and share channels while keeping
  separate cards.
- The bot and Discord-server identity cannot change while stored game-server
  destinations exist. Disabling destinations preserves them. Token rotation for
  the same bot is supported.
- Custom scalar and flat-list state requires a reporting mod. See
  [Game integrations](/Campfire/game-integrations/); there is no separate structured
  player-roster protocol.
- Discord commands and inbound chat are not supported.
