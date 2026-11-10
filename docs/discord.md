---
title: Discord integration
description: Connect a Discord bot, publish game-server notifications and cards, and recover delivery after an outage.
---

Campfire can send game-server events to Discord and maintain a live server card.
This is separate from personal inboxes and Apprise monitoring delivery. See the
[notification and card reference](../notifications/#discord-bot-setup) for event
filters, card fields, and freshness behavior.

## Connect the bot

1. Create an application and bot in the
   [Discord developer portal](https://discord.com/developers/applications).
   Discord's
   [bot authorization flow](https://docs.discord.com/developers/topics/oauth2#bot-authorization-flow)
   uses the `bot` scope.
2. Give the bot **View Channel**, **Send Messages**, **Embed Links**, **Attach Files**, and **Read
   Message History** in each destination text channel. Channel overrides can
   remove permissions granted by a role. Campfire checks the effective
   [Discord permissions](https://docs.discord.com/developers/topics/permissions)
   and explains missing permissions in its channel selector.
3. Sign in to Campfire as an administrator. Open **Integrations** from the
   account menu, choose **Add integration**, and select **Discord**. Enter the
   bot token; available Discord servers load automatically. Use **Add bot to
   Discord** beside the server selector to install it with the required permissions.
   Returning to Campfire reloads available servers. Select one and save.
4. Edit the saved connection, expand **Test delivery**, choose a channel, and
   send a test message. This checks
   the saved connection before configuring game servers.
5. Open a game's console and choose **Actions → Edit server → Integrations → Add integration**.
   Select a saved integration; a sole integration is preselected in the dropdown
   above the channel. Choose a channel, enable the
   destination, and select notification categories and optional card fields.
   Add more destinations for different
   audiences, such as backups in an admin channel and the server card in a public
   channel. Click a destination row to edit it. Card publishing works with all
   notification categories unchecked.
   Existing entries retain their selected integration when edited. Finish the
   wizard with **Save changes** to apply additions, edits, and removals together
   with server settings. **Cancel** discards integration drafts.
   Integration changes commit as one batch, including channel swaps. If a save
   response is lost, retrying the same drafts does not duplicate integrations.
6. Reopen the integration settings to confirm **Card publishing** reaches
   **Published**, and check matching messages in Discord. Enabling an integration
   does not replay old activity. Open the server's **Activity** tab and choose
   **Notifications** to inspect delivery outcomes or retry a failed message.

Global setup is administrator-only. Click a saved connection row to edit it.
Per-server configuration requires
`server.configure` and its prerequisite grants. Console-alert sharing
also needs console-read access; backup-event sharing needs backup-read access.
Owners, administrators, and server editors can configure integrations.

## Game screenshots and clips

Game-event subscriptions also forward their image and video uploads as native
Discord attachments. Each completed upload follows the event as its own message
with the same title and fields, and uses the same event-type and field filters.
Files arriving after the text notification are still delivered, and upload
retries do not create duplicate attachment messages. Existing activity is not
replayed when a destination is enabled.

Files are uploaded directly from retained game media; Discord does not need
access to authenticated panel URLs. Campfire uses Discord's default 20 MiB
per-file allowance. Larger files stay available in Activity and produce a
notification explaining the limit. If activity retention removes a file before
delivery, its notification explains that the attachment has expired. Files already
uploaded to Discord remain there independently of panel retention.

The mock Discord viewer displays the same uploads as images and playable videos,
with download links. It stores disposable copies separately from panel media and
removes them when their mock message is deleted or the development session resets.

## Delivery and card recovery

| What you see                            | What to do                                                                                                     |
| --------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| No Discord servers in setup             | Check that the bot was installed in the intended Discord server, then reopen its form.                         |
| An unavailable channel                  | Fix the bot's role or channel overrides, reopen the server's Integrations drawer, and save the destination.    |
| Failed card                             | Fix the reported issue, then choose **Retry card**.                                                            |
| Unconfirmed card                        | Choose **Check card** to search for the card whose creation response was lost.                                 |
| A deliberately deleted card             | Leave the binding enabled. Campfire checks quiet cards periodically and recreates a confirmed missing message. |
| Cleanup pending after a move or disable | Restore the bot's access to the old channel and retry. The saved reference is retained until cleanup succeeds. |

Notification status and manual recovery remain available through the per-binding
messages API; there is no notification-history interface in the panel yet.
Transient delivery failures retry automatically with backoff. Rate-limit
responses defer work for the requested cooldown. Known failures eventually stop
retrying and remain visible. Credential failures pause the connection; channel
failures pause the affected destination.

Notification history retains completed or failed messages for up to 30 days,
with a maximum of 10,000 per destination. After that, Campfire removes their
contents and keeps compact event receipts for deduplication for up to 90 days,
with a maximum of 100,000 per destination. Replaying an event after its receipt
expires can send it again. Pending work, unconfirmed sends, permission pauses,
and live-card references are retained until resolved or the integration is removed.
Cleanup runs at panel startup and hourly thereafter.

Delivery workers share checked Discord permissions for up to 30 seconds to avoid
repeating setup requests for every notification. Explicit setup and channel
checks stay fresh; Discord authorization failures invalidate the worker cache.

An unconfirmed send may already exist in Discord. Receipt checks are bounded to
recent history, and unresolved creation remains paused to avoid duplicates. Keep
the saved message identities during recovery. Repeated checks can find a receipt
that becomes visible later; they never authorize a replacement creation.

Changing channels preserves the old card's identity until it is removed. A
permission failure or timeout does not authorize a new card in the new channel.
Disabling the card, binding, or global connection also requests cleanup.
Disabling a card or removing notification subscriptions does not require
publishing permission; cleanup can finish after Discord access returns.
Before deleting a game server or removing its bot from Discord, disable its card and
save the change, and wait for the card to disappear from Discord so cleanup can
finish while its binding and credentials still exist.

## Rotate a token

Edit the connection in the global Integrations drawer and choose **Change bot
token** to replace a revoked or rotated token. The saved token stays hidden;
available servers load automatically using it when editing. After saving, send
a test message. Verify affected cards and retry any known
failed notifications you still want delivered.

Tokens are never returned by the setup API. Campfire encrypts them in its private
state directory and retires the old credential after a replacement is committed.
Disabling an unchanged connection remains possible while Discord is unavailable.
A setup save that became stale after another administrator changed the connection
must be reloaded before it can be submitted again.

The connection's enable toggle pauses delivery without discarding its settings.
**Remove integration** disconnects its game servers and removes the saved token,
bindings, and delivery records from Campfire. Messages already sent to Discord
remain there as history. Removal works while Discord is unavailable.

## Back up and restore

Preserve the panel database and the `integration-secrets` directory under the
panel's state directory as a matching backup. That directory contains both the
encryption key and encrypted token files. A database backup alone is insufficient.

1. Stop the panel and take a consistent database and state backup, following
   [Upgrades and recovery](../operations/). For PostgreSQL, include its database
   backup as well as the filesystem state.
2. Restore the matching database, key, and encrypted credential files together.
   Preserve private permissions and ownership so the panel process can read
   them. Key and token files must remain readable only by their owner.
3. Start the panel, check the saved connection, and send a test message. Inspect
   each server's publishing and delivery status. Pending work and saved card
   references survive a restart.

If the key is missing or damaged, restore it from the matching state backup.
Campfire does not replace a missing key while encrypted credentials already
exist. Avoid clearing message identities during recovery: an interrupted send
may have succeeded before its response was lost.

## Initial-version scope

- One global Discord bot and Discord server are supported. A game server can have
  several destination channels with independent notifications, filters, cards,
  and history. Several game servers can share a channel and retain separate cards.
- The bot and Discord-server identity cannot be changed while stored game-server
  bindings exist. Disabling bindings preserves them. Token rotation for the
  same bot remains supported.
- Online names and invite codes can be selected when a mod reports them as
  scalar game fields. A structured player roster is a separate extension; see
  [Game integrations](../game-integrations/).
- Discord commands and inbound chat are outside this version's scope.
