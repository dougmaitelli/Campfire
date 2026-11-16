---
title: Game integrations
description: Build game mods and adapters that publish state, events, screenshots, and clips through Spark.
---

Game mods, plugins, and adapters publish through a private Spark Unix socket in
the game container. Spark supplies the server identity and forwards publications
to Campfire. Publishers do not need panel API tokens, Spark certificates, or a
public network endpoint.

Current state appears in server readings; events and media appear in
**Activity → Game events**. [Discord destinations](/Campfire/discord/) can forward
matching events and media and display selected state fields on live cards.

## Use a game integration

For server operators, a compatible mod handles the publishing API described below.

1. Install a publisher using its own instructions. The
   [Campfire Valheim plugin](https://github.com/dougmaitelli/campfire-plugin-valheim)
   is one available integration; review its requirements and privacy settings.
2. Open **More server actions → Edit server**, enable **Enable game integration**,
   and save.
3. Restart the game through Campfire so its container receives the private socket.
4. Check server readings and **Activity → Game events**. Available fields and
   events depend on the mod.
5. Optionally configure a [Discord destination](/Campfire/discord/). Enabling
   publishing alone does not send data to Discord.

If data is missing, check the feature setting, container restart, mod logs, and
Spark connection. Expired or unavailable state does not mean zero players.

## Connect and register

### Socket discovery

At startup, read `CAMPFIRE_GAME_SOCKET`. If absent, leave the integration inactive
rather than failing the game. Use an HTTP client supporting `AF_UNIX` stream
sockets, or an adapter process in the same container. No Campfire SDK is required.

Use the supplied socket path exactly. After a Spark restart, reconnect to that
path if an existing connection fails. The hostname in `http://spark/v1/publish`
is only an HTTP host value: requests travel through the socket, not DNS or TCP.
Do not send them to the panel's public URL or `/api/v1` endpoints.

| Method and path                                   | Request                          | Success                                                                             |
| ------------------------------------------------- | -------------------------------- | ----------------------------------------------------------------------------------- |
| `PUT /v1/registration`                            | Label-registration JSON envelope | `204`: definitions durably stored in Campfire                                       |
| `POST /v1/publish`                                | One state or event JSON envelope | State: `204`, snapshot accepted in memory; event: `202`, persisted in Spark's queue |
| `GET /v1/events/queue`                            | No body                          | `200` with queue-status JSON                                                        |
| `PUT /v1/events/{event_id}/media/{attachment_id}` | Raw bytes and media headers      | `204`: attachment committed in Campfire                                             |

Success responses other than queue status have no body. Errors are plain text;
use HTTP status and `Retry-After`, not error wording, to decide how to recover.

### Startup registration and labels

Before publishing, register field labels with `PUT /v1/registration` and
`Content-Type: application/json`. Registration creates no Activity entry.

```json
{
  "version": 1,
  "registration": {
    "revision": 1,
    "state_fields": {
      "game_mode": "Game mode",
      "player_names": "Online players"
    },
    "event_fields": {
      "player.joined": { "player": "Player name" }
    }
  }
}
```

Save this as `registration.json` and send it from the game container:

```sh
curl --unix-socket "$CAMPFIRE_GAME_SOCKET" \
  --include --max-time 12 --request PUT \
  --header 'Content-Type: application/json' \
  --data-binary @registration.json \
  http://spark/v1/registration
```

| Property                    | Rules                                                                                          |
| --------------------------- | ---------------------------------------------------------------------------------------------- |
| `version`                   | Integer `1`                                                                                    |
| `registration.revision`     | Positive, monotonically increasing integer, at most `9007199254740991`                         |
| `registration.state_fields` | Optional map of state field keys to labels; at most 32 entries                                 |
| `registration.event_fields` | Optional map of event types to field-label maps; at most 32 event types and 32 fields per type |

Field keys and event types follow the [field-name rules](#field-types-and-limits).
Labels are nonempty UTF-8 strings of at most 128 bytes, without control characters
or leading/trailing whitespace. They are plain text, not HTML. The envelope must
fit within 16 KiB. Registration defines labels, not values, so fields can be
registered before they are published.

Repeating identical definitions at the same revision is idempotent. Changed
definitions require a higher revision; a stale revision or different definitions
at the same revision return `409`. A newer manifest replaces all definitions:
omitted definitions are removed, and an empty manifest is valid.

Wait for `204` before publishing. Registration is not queued offline; retry `503`
with bounded backoff. Register at each adapter startup. Definitions persist across
panel and Spark restarts until replaced or the server is deleted.

Registered labels are authoritative; publications need only field keys and values.
Unregistered keys display exactly as supplied. Labels do not change filter or
card-selection identities. Updated registrations can rename fields in retained
history, but do not rewrite messages already sent to Discord. Campfire supplies
its own labels for built-in `players` and `capacity`.

#### Inline-label compatibility

Publishers without registration may send `state.field_labels` or
`event.field_labels`, mapping keys to labels using the same label limits. Each
labeled key must exist in that publication's `fields` or `data`. Registered
servers ignore inline labels in favor of their manifest.

Without registration, state labels must be repeated in each complete snapshot;
omitted labels fall back to the key. Event labels update a persistent dictionary
per server, event type, and field key. An omitted event label preserves the known
label; publishing the key as its label resets its display text. Replaying an
existing event ID does not update labels. Retention cleanup removes unused event
label definitions.

Activity filters retain keys as identity. When multiple event types disagree on
a field label, discovery falls back to the key. Without retained events containing
a field, it disappears from discovery. Prefer registration for new publishers.

### Host setup and capabilities

Publishing is opt-in per server. After enabling it, a restart through Campfire
adds a read-only directory mount at `/run/campfire-game` and sets
`CAMPFIRE_GAME_SOCKET=/run/campfire-game/publish.sock`. The read-only mount permits
socket connections; it does not expose host files or Spark credentials. Disabling
publishing revokes access when Spark receives the configuration. Restart the game
to remove the mount.

Spark recreates listeners after its own restart. Keep its data directory private,
on the host filesystem visible to Docker, and use a short absolute `--data` path:
Linux Unix-socket paths are typically limited to 107 bytes. Host administrators
can access the sockets. Requests are checked against local configuration and
Campfire's authoritative server placement.

The local API version is separate from capabilities negotiated between Spark and
Campfire. A socket connection alone does not guarantee every operation is supported.

| Capability                    | Required for                                              |
| ----------------------------- | --------------------------------------------------------- |
| `server.game-integration.v1`  | Base state and event transport                            |
| `server.game-registration.v1` | Revision-checked label registration                       |
| `server.game-state-lists.v1`  | Flat-list state fields                                    |
| `server.game-events.v1`       | Durable event queues and committed panel acknowledgements |
| `server.game-media.v1`        | Media uploads                                             |

Keep both Spark and Campfire compatible with the features your publisher uses.
Unsupported registration, durable events, or media return `503`. A Spark that
supports lists returns `503` if Campfire does not; a Spark without list support
rejects them with `400`. Publishers without these features can continue using
supported scalar state and inline labels. Spark versions that do not recognize
inline-label properties reject them.

## Publish data

Send `POST /v1/publish` with `Content-Type: application/json` and a version 1
JSON envelope containing exactly one `state` or `event`. The socket determines
the server: do not include a server ID. Do not batch envelopes or send
newline-delimited JSON. Unknown envelope, state, and event properties are rejected;
custom values belong in `state.fields` or `event.data`.

### State snapshots

```json
{
  "version": 1,
  "state": {
    "players": 2,
    "capacity": 20,
    "fields": {
      "game_mode": "survival",
      "player_names": ["Alex", "Sam"]
    }
  }
}
```

Save as `state.json` and publish:

```sh
curl --unix-socket "$CAMPFIRE_GAME_SOCKET" \
  --include --max-time 12 \
  --header 'Content-Type: application/json' \
  --data-binary @state.json http://spark/v1/publish
```

| Property              | Rules                                                 |
| --------------------- | ----------------------------------------------------- |
| `players`, `capacity` | Optional integers from `0` to `1000000`               |
| `fields`              | Optional map of up to 32 scalar or flat-list values   |
| `field_labels`        | Optional inline-label map for unregistered publishers |

State is a complete replacement: omitted fields clear previous values, and
`{"version":1,"state":{}}` clears the snapshot. Omit unknown player counts;
use `0` only when the count is known to be zero. Lists do not determine the
built-in player count—publish `players` separately. State has no event ID or
producer timestamp.

Publish a complete snapshot periodically, for example every 15 seconds. State
expires after 60 seconds without an update; events do not refresh it. Only the
latest snapshot is stored in memory, with no history, and a panel restart clears
it. While disconnected, keep only the newest unsent snapshot.

Stopped, disconnected, or unknown runtime state suppresses game values. Lifecycle
operations invalidate state and temporarily reject publications until a healthy,
non-busy running observation reopens ingestion. Retry with bounded backoff. Stops
or restarts outside Campfire take effect on the next runtime observation.

### Events

```json
{
  "version": 1,
  "event": {
    "id": "join-unique-id",
    "type": "player.joined",
    "occurred_at": "2026-10-24T00:00:00Z",
    "title": "Alex joined",
    "data": { "player": "Alex" }
  }
}
```

Send the event with the same curl request, substituting `event.json`. Generate
its ID and timestamp once when the event happens, not on each request attempt.

| Property       | Rules                                                                      |
| -------------- | -------------------------------------------------------------------------- |
| `id`           | Required string, 1–128 bytes; unique within this server's retained history |
| `type`         | Required name following the field-name pattern; namespacing is recommended |
| `occurred_at`  | Required nonzero RFC 3339 timestamp; UTC with `Z` is recommended           |
| `title`        | Required string, 1–256 bytes                                               |
| `data`         | Optional map of up to 32 scalar values; no lists                           |
| `field_labels` | Optional inline-label map for unregistered publishers                      |

A UUID is a suitable event ID. Retain the entire envelope unchanged across retries.
Campfire deduplicates per server within retained history; the first stored payload
wins. Reusing a queued ID with different content returns `409`. Different servers
may use the same ID; it is not an authorization token.

Types such as `player.joined`, `game.announcement`, or `valheim.boss_defeated`
are supplied by your integration. Campfire imposes no round or match model and
does not translate event types into game commands.

### Field types and limits

| Item                        | Allowed values                                                     |
| --------------------------- | ------------------------------------------------------------------ |
| Field names and event types | `^[a-z][a-z0-9_.-]{0,63}$`                                         |
| Field map                   | At most 32 entries                                                 |
| Scalar                      | String up to 1,024 UTF-8 bytes, finite JSON number, or boolean     |
| State list                  | At most 64 scalar elements; empty and mixed-type lists are allowed |
| Event data                  | Scalars only                                                       |
| JSON envelope               | At most 16 KiB, including whitespace, labels, and list elements    |

Objects, nested lists, and null values—including null list elements—are rejected.
Lengths are byte limits, not character counts. Use strings for opaque IDs or exact
large integers because JSON numbers are decoded as floating-point values. Publish
bounded summaries rather than unbounded collections.

Lists preserve order and duplicates and display as comma-separated values. Blank
strings are omitted from display; an empty or all-blank list displays **None**.
Omit a field when its value is unknown. List state is transient, not a retained
event or offline queue. Player names are visible to users with server-view access
and to Discord audiences when selected for a card; publish only appropriate data.
See [live server cards](/Campfire/discord/#live-server-cards) for display choices.

## Screenshots and clips

Publish an event first, then upload raw bytes through the same socket:

```sh
curl --unix-socket "$CAMPFIRE_GAME_SOCKET" \
  --request PUT --include --max-time 125 \
  --header 'Content-Type: image/png' \
  --header 'X-Campfire-Media-Name: world.png' \
  --data-binary @world.png \
  http://spark/v1/events/join-unique-id/media/screenshot-1
```

Use the **producer event ID**, URL-encoded as one path segment, and a stable
attachment ID. Encode each segment separately. Send raw bytes, not multipart,
base64, or JSON. Set `Content-Length` when known; streamed bodies have the same
limits.

| Item                              | Requirement or limit                                                                                           |
| --------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| Attachment ID                     | 1–64 ASCII letters, digits, underscores, or hyphens                                                            |
| `X-Campfire-Media-Name`           | Filename, 1–128 UTF-8 bytes; no path separators or control characters                                          |
| `Content-Type`                    | Exactly `image/png`, `image/jpeg`, `video/mp4`, or `video/webm`, without parameters; must match file signature |
| Image dimensions                  | Valid image header, at most 40 million pixels                                                                  |
| Single attachment                 | 25 MiB                                                                                                         |
| Attachments per event             | 4                                                                                                              |
| Retained media per server / panel | 250 MiB / 1 GiB                                                                                                |
| Concurrent uploads                | 2 per Spark and 2 across Campfire                                                                              |
| Upload deadline                   | 2 minutes                                                                                                      |

Clips are stored without transcoding; playback depends on browser codec support.
Media is streamed to Campfire using Spark's identity and is not queued locally.
`204` confirms committed metadata and bytes. Retry a lost response with the same
attachment ID, name, MIME type, and bytes; identical retries succeed without
adding another attachment, while changed content returns `409`.

The event's `202` can precede arrival at Campfire. Media returns `409` with
`Retry-After: 5` until the event is retained. Keep the file for retry, subject to a
bounded retry budget. See [error handling](#delivery-and-error-handling) for other
responses. Failed ingestion exposes no partial attachment.

Attachments expire with their parent event. Deletion releases logical quota but
does not guarantee immediate shrinking of database files; database/WAL space and
backups can exceed the logical media budget. Discord destinations can forward
attachments, subject to [Discord delivery limits](/Campfire/discord/#game-screenshots-and-clips).

## Delivery and error handling

### Event queue and retention

`202` means Spark persisted the event locally, not that Campfire received it.
The queue survives Spark restarts; entries are removed after a committed panel
acknowledgement. Delivery retries use the same event ID.

| Queue property                        | Limit or behavior                                                           |
| ------------------------------------- | --------------------------------------------------------------------------- |
| Capacity                              | 512 events per server; 4,096 per Spark                                      |
| Delivery lifetime                     | 24 hours                                                                    |
| Full queue                            | Rejects new events with `429` and `Retry-After: 5`; existing entries remain |
| Transient delivery failure            | Backoff up to 30 seconds; reconnect triggers an immediate retry             |
| Expired or permanently rejected event | Dropped with a warning in Spark logs                                        |

Deletion or reassignment of a server can permanently reject queued events. Delivery
is not strictly ordered: a delayed retry can arrive after a newer event. Include
a game-specific sequence in `data` if needed. Delivery is neither unlimited nor
an exactly-once guarantee.

Inspect the server's queue:

```sh
curl --unix-socket "$CAMPFIRE_GAME_SOCKET" http://spark/v1/events/queue
```

```json
{
  "pending": 1,
  "limit": 512,
  "node_limit": 4096,
  "dropped": 0,
  "last_error": ""
}
```

Counters and errors are local to the Spark process; pending entries persist.
A corrupt or oversized spool file blocks queue startup rather than silently
removing accepted data. Queue status is aggregate, not an event receipt:
`pending: 0` can mean delivered or dropped. Verify individual events in retained
history.

Campfire retains up to 10,000 events per server for up to 30 days by default,
using panel receipt time rather than the game's clock. Configure these limits in
[Settings reference](/Campfire/configuration/#activity-and-game-event-retention).
Once an event expires, its ID leaves the deduplication window. Server deletion
removes its retained events and attachments.

### Request limits and responses

Each server permits 10 JSON publications per one-second window, with at most 32
in-flight publications across Spark. State waits up to eight seconds for panel
acknowledgement. Media has separate upload slots; queue inspection is a separate
GET route.

| Outcome                             | Producer action                                                                                                                                         |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Missing socket, reset, or timeout   | Reconnect later. Retry an unchanged event or attachment; send only the newest state snapshot.                                                           |
| `202` event                         | Stop resubmitting; Spark owns its bounded queued copy.                                                                                                  |
| `204` registration, state, or media | The panel operation succeeded.                                                                                                                          |
| `400`                               | Correct schema, metadata, encoding, or content.                                                                                                         |
| `401` media                         | Check Spark's panel authentication; do not add a game API token.                                                                                        |
| `403`                               | Stop publishing and report revoked configuration or server placement.                                                                                   |
| `404` / `405`                       | Check the local `/v1` route and HTTP method.                                                                                                            |
| `409` registration                  | Use the original manifest or a higher revision for changed definitions.                                                                                 |
| `409` event                         | Restore the original queued payload, or use a new ID for a distinct event.                                                                              |
| `409` media with `Retry-After`      | Wait for the parent event to reach retained history, then retry unchanged.                                                                              |
| `409` media without `Retry-After`   | The attachment ID has different content or metadata; do not overwrite it.                                                                               |
| `413`                               | Correct request size, or media attachment-count/storage limits; do not retry indefinitely.                                                              |
| `429`                               | Apply backpressure and respect `Retry-After`: publication rate limits use one second, full event queues use five seconds. Media slots can also be busy. |
| `503`                               | Retry with bounded backoff; persistent failures need connection, capability, runtime, or Spark-log investigation.                                       |

Log the HTTP status and bounded error text. Use capped exponential backoff with
jitter and a finite retry budget; respect `Retry-After` whenever supplied.

### Publisher lifecycle

Capture state through the game's supported APIs, then hand it to a background
worker. Never perform socket I/O, uploads, sleeps, or retries on the simulation
thread.

1. Discover the socket and register labels before publishing.
2. Send periodic complete state snapshots, keeping only the newest unsent state.
3. Create each event envelope once. If loss before Spark accepts it matters,
   persist it in a bounded producer outbox.
4. Remove the outbox copy after `202`, acknowledging Spark's queue lifetime and
   placement limits.
5. Retain attachment bytes and metadata until `204` or your retry budget expires.
6. Use a short, bounded shutdown flush; never block game shutdown indefinitely.

Rate-limit normal traffic so event bursts do not starve state heartbeats. Media
support is optional; state/event publishers need not implement attachments.

## Read published data

These are authenticated **panel** API endpoints for readers, not publisher routes.
They require server-view access and applicable token scopes. Mods publishing
through Spark do not need to call them.

### Current state

`GET /api/v1/monitoring` returns an optional `game` object on each authorized
server, with `updated_at`, `fresh_for_seconds`, and `status` (`fresh`, `stale`, or
`unavailable`). Only fresh, runtime-valid responses include `state`. Servers that
have never published omit `game`.

### Retained events

`GET /api/v1/servers/{id}/events` uses the shared numbered-page response:

```json
{
  "items": [],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total_items": 0,
    "total_pages": 0
  }
}
```

Results are ordered by panel receipt ID, newest first. Totals include only matching,
authorized records. Out-of-range pages return an empty `items` array with the
requested page number and current totals. New events or retention deletions can
shift numbered page boundaries; this is not a snapshot. The old `limit` and
`before` parameters are rejected rather than silently ignored.

| Query parameter | Meaning                                                         |
| --------------- | --------------------------------------------------------------- |
| `page_size`     | 1–100; default 50; invalid sizes are rejected                   |
| `page`          | One-based page number; default 1                                |
| `type`          | Exact event type                                                |
| `search`        | Text in title or event data, at most 1,024 bytes                |
| `data.<field>`  | Whole scalar value; empty matches any value for that field      |
| `from`, `to`    | Occurrence time in Unix seconds; inclusive start, exclusive end |

Use `true`/`false` for booleans and numeric text for numbers. Field names follow
publication rules, each name may appear once, and up to 32 field conditions are
allowed. Values are limited to 1,024 bytes. Conditions combine before pagination.
For example, `?search=Alex&data.player=Alex` combines text and field filtering.
The Activity UI's time filters use panel-recorded time, not these occurrence-time
parameters.

Events include producer `event_id`, type, title, occurrence/receipt times, source
server and Spark IDs, scalar data, and attachment metadata.
`GET /api/v1/servers/{id}/events/filters` returns sorted distinct `types` and a
`fields` map of observed scalar values as strings, across the server's retained
history independently of filters or page position.

### Retained media

`GET /api/v1/servers/{id}/events/{event}/media/{attachment}` uses the **panel's
numeric event ID**, not the producer ID used for uploads. Attachment IDs remain
the producer-assigned IDs. The endpoint supports HTTP ranges and `?download=1`.
Responses are private and non-cacheable, not public media links.

## Test an integration

Use the curl requests from inside an opted-in game container with a real Spark,
then verify state and Activity in Campfire. Test at least these cases:

- Clear state with an empty snapshot and omit a previously supplied field.
- Retry an event unchanged and verify one retained entry.
- Disconnect the panel: state should fail transiently while supported events queue.
- Restart Spark with pending events, reconnect to its socket, and verify delivery.
- Stop heartbeats for more than 60 seconds and confirm state becomes unavailable.
- Upload media immediately after `202`, handle a possible `409`, and retry unchanged.
- Shut down during failed publication without blocking the game thread.

For failures, use the [response table](#request-limits-and-responses),
`journalctl -u spark`, and Campfire logs. A missing socket usually means the feature
is disabled, the container needs recreation, or Spark is unavailable. Do not add
administrator tokens or Spark private keys to work around placement errors.

### Example publisher

The [Node.js publisher](https://github.com/dougmaitelli/Campfire/blob/master/scripts/game-publisher.mjs)
uses built-in modules to register labels and send one state snapshot or event.
Copy it into a game container with Node.js:

```sh
node game-publisher.mjs state
node game-publisher.mjs event
```

It discovers `CAMPFIRE_GAME_SOCKET`; host-side development can supply the specific
server's socket path as the second argument. It makes one attempt, not a production
retry/outbox implementation. Other runtimes can use the same HTTP contract with
their own Unix-socket-capable clients.

For panel development without a real game, use the
[mock publisher](/Campfire/development/#game-publications-and-alerts). It does not
replace testing against the real Spark socket.

### Implementation references

- [Publication schemas and limits](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/shared/model/game_integration.go)
- [Local HTTP routes](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/spark/agent/game_integration.go)
- [Event queue](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/spark/agent/game_event_queue.go)
- [Media schemas](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/shared/model/game_media.go)
- [Media forwarding](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/spark/agent/game_media.go)
- [Panel placement and freshness checks](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/campfire/service/game_integration.go)
