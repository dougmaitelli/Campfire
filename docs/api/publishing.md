---
title: Notifications and publishing API
description: Read an inbox, retrieve public availability, and manage Discord connections and destinations.
---

Paths are relative to `/api/v1`. Use public status pages for information your deployment has chosen to publish. Notification inboxes and Discord settings require authentication.

## Public status pages

| Method and path                   | Access                 | Request and result                                                                                                                    |
| --------------------------------- | ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /public/status-pages/{slug}` | Public                 | Returns `title`, `collected_at`, `poll_seconds`, and ordered `entries`. Unknown slugs return `404`.                                   |
| `GET /status-pages`               | Administrator with `*` | List configured pages, including their management IDs and selected servers.                                                           |
| `POST /status-pages`              | Administrator with `*` | Send `title`, `slug`, and ordered `entries` of `{"server_id":"…","name":"…"}`. Publishes immediately and returns `201` with the page. |
| `PUT /status-pages/{pageID}`      | Administrator with `*` | Replace the title, slug, and ordered entries. Returns the updated page.                                                               |
| `DELETE /status-pages/{pageID}`   | Administrator with `*` | Remove the publication. Returns `204`.                                                                                                |

For a page with the slug `community`, request `GET /api/v1/public/status-pages/community` without an `Authorization` header.

A public entry reports `online`, `offline`, or `unknown` and includes availability history. It can also include a public game address, artwork, and game-state readings. Use the returned `poll_seconds` to schedule refreshes. On `429`, honor `Retry-After`. See [public status pages](/Campfire/status-pages/) for how readiness and stale observations affect the result.

## Notifications

| Method and path                            | Required scope         | Request and result                                                                                    |
| ------------------------------------------ | ---------------------- | ----------------------------------------------------------------------------------------------------- |
| `GET /notifications`                       | `notifications.read`   | Page of the account's visible notifications. Add `unread=true` to filter.                             |
| `GET /notifications/unread`                | `notifications.read`   | Returns `{"unread":3}` when three visible messages are unread.                                        |
| `PUT /notifications/{notificationID}/read` | `notifications.write`  | Send `{"read":true}` or `{"read":false}`. Returns `204`.                                              |
| `POST /notifications/read-all`             | `notifications.write`  | Send `{"through_id":123}` using the highest notification ID you intend to acknowledge. Returns `204`. |
| `GET /notifications/status`                | `notifications.manage` | Delivery counts, resource thresholds, sampling interval, and Apprise availability.                    |
| `POST /notifications/test`                 | `notifications.manage` | Generate a test notification. Returns `202` with `notification_id` and `apprise_queued`.              |
| `POST /notifications/retry-deliveries`     | `notifications.manage` | Requeue failed external deliveries.                                                                   |

Notification access also depends on the target. Include `server.view` to read server notifications for accessible servers and `sparks.manage` for Spark notifications when authorized. Marking through a known ID lets messages received after the read stay unread.

## Discord connection

These routes manage the panel-wide connection. Each requires an administrator with a full-access token (`*`). See [Discord setup](/Campfire/discord/) for the bot and channel permissions.

| Method and path                      | Request and result                                                                                           |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------ |
| `GET /integrations/discord`          | Read configuration and connection health.                                                                    |
| `POST /integrations/discord/preview` | Send the Discord bot `token` to look up the bot and available guilds before saving.                          |
| `PUT /integrations/discord`          | Send `enabled`, `guild_id`, and the Discord `token` when configuring credentials. Returns connection status. |
| `DELETE /integrations/discord`       | Remove the saved connection. Returns `204`.                                                                  |
| `GET /integrations/discord/channels` | List eligible channels from the configured guild.                                                            |
| `POST /integrations/discord/test`    | Send `{"channel_id":"…"}` to post a test message. Returns `204`.                                             |

## Per-server Discord destinations

All routes below require `server.configure` on the server. Publishing each chosen source also requires its underlying permission. A binding identifies one destination. Its editable fields are `integration_id`, `channel_id`, `enabled`, `subscriptions`, and `card`. Read the server's integration list to discover available connections, sources, and existing bindings.

| Method and path                                                                  | Request and result                                                                                      |
| -------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `GET /servers/{id}/integrations/discord`                                         | Returns `connections`, `sources`, binding `items`, and available field labels.                          |
| `POST /servers/{id}/integrations/discord`                                        | Send binding settings. Returns `201` with the saved binding.                                            |
| `PUT /servers/{id}/integrations/discord`                                         | Apply a batch with `upserts` (each has `id` and `settings`) and `remove_ids`. Returns the updated list. |
| `GET /servers/{id}/integrations/discord/{bindingID}`                             | Read one binding and its status.                                                                        |
| `PUT /servers/{id}/integrations/discord/{bindingID}`                             | Replace the editable binding settings. Returns the saved binding.                                       |
| `DELETE /servers/{id}/integrations/discord/{bindingID}`                          | Remove a single destination.                                                                            |
| `GET /servers/{id}/integrations/discord/channels`                                | Use `integration_id` to list channels for the chosen connection.                                        |
| `GET /servers/{id}/integrations/discord/card`                                    | Preview the current and expired card. Repeat the `field` query parameter for selected readings.         |
| `POST /servers/{id}/integrations/discord/{bindingID}/card/retry`                 | Send `{}` to retry the destination's live card. Returns `204`.                                          |
| `GET /servers/{id}/integrations/discord/{bindingID}/messages`                    | Page of delivery history. Optional `state` filter.                                                      |
| `POST /servers/{id}/integrations/discord/{bindingID}/messages/{messageID}/retry` | Send `{}` to retry an eligible message delivery. Returns `204`.                                         |
