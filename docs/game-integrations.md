---
title: Game integrations
description: Build game mods and adapters that publish state, events, screenshots, and clips through Spark.
---

This guide describes the version 1 publishing API for game mods, plugins, and
adapters. A publisher sends HTTP requests to its game's local Spark Unix socket.
Spark supplies the server identity and forwards the data to Campfire. Your mod
does not connect to the panel directly, implement the Spark protocol, or receive
panel API tokens or Spark certificates.

State publications appear in the overview, server list, and server operations
page. Campfire keeps only the latest snapshot per server in memory. There are no
game-state charts or history, and a Campfire restart clears these snapshots.
Events are durably retained in the database and appear in each server's
**Game activity** tab, with optional screenshots and clips. Forwarding game
publications to Discord is not implemented.

## Communication model

```text
Game mod or adapter
    |
    | HTTP over its private Unix socket
    v
Spark on the game host
    |-- state/events --> existing client-certificate-authenticated WebSocket
    `-- media --------> HTTPS with the same Spark client identity
                             |
                             v
                    Campfire agent listener
                        |-- state --> latest snapshot in memory
                        |-- events/media --> panel database
                        `-- authorized panel API --> browser or API client
```

For JSON publications, Spark checks the payload and local server configuration,
adds the server ID, and sends a `game.publish` message over its existing panel
connection. Campfire verifies that this Spark still hosts the server and that
game integration is enabled. Its `game.ack` response confirms either snapshot
ingestion or a committed event transaction. Spark handles these messages and
their correlation IDs; they are not part of the mod's HTTP API.

The local acknowledgement depends on what you send:

| Publication | Local success    | What success guarantees                                                            |
| ----------- | ---------------- | ---------------------------------------------------------------------------------- |
| State       | `204 No Content` | Campfire accepted this complete snapshot into memory.                              |
| Event       | `202 Accepted`   | Spark persisted the event in its local queue; panel delivery may still be pending. |
| Media       | `204 No Content` | Campfire committed the attachment's metadata and bytes.                            |

State is not queued while disconnected. Events can queue while the panel is
unreachable, subject to capacity and lifetime limits. Media is streamed directly
and must be retried by the producer if interrupted.

## Mod startup and endpoint discovery

The integration needs an HTTP client that can connect to an `AF_UNIX` stream
socket. A mod can use its runtime's Unix-socket support or delegate publishing to
an adapter process inside the same game container. No Campfire-specific SDK is
required.

At startup, read `CAMPFIRE_GAME_SOCKET`. When it is absent, leave the integration
inactive rather than failing the game server. When it is present, use that exact
path; do not derive host paths, server IDs, or the panel address. After a Spark
restart, an existing connection may fail even though the socket path is unchanged.
Reconnect to the path on the next attempt.

The hostname in examples such as `http://spark/v1/publish` is just an HTTP host
value. The connection goes through the configured socket, not DNS or TCP. Do not
send these requests to the panel's public URL or its `/api/v1` endpoints.

The publisher interface has three routes:

| Method and path                                   | Request                                | Success response                              |
| ------------------------------------------------- | -------------------------------------- | --------------------------------------------- |
| `POST /v1/publish`                                | One JSON state or event envelope       | Empty `204` for state; empty `202` for events |
| `GET /v1/events/queue`                            | No body                                | `200` with queue-status JSON                  |
| `PUT /v1/events/{event_id}/media/{attachment_id}` | Raw attachment bytes and media headers | Empty `204`                                   |

Use `Content-Type: application/json` for publications. Media requires its exact
supported MIME type. Publication errors are plain text; do not assume a JSON
error body or parse error wording as a stable error code. Use the HTTP status and
`Retry-After` header to decide what to do next.

## Enable publishing

In the dashboard, edit a game server and select **Enable game integration**.
Publishing is disabled by default for each server. Spark automatically advertises
support as `server.game-integration.v1`; no host flag or environment variable is
required. Durable event delivery additionally negotiates `server.game-events.v1`.
An older Campfire can still receive supported state snapshots, but Spark will not
send queued events through the older, non-durable ingestion contract.

The local `/v1` API and JSON `version` are separate from the Spark wire-protocol
version. Spark and Campfire negotiate support themselves:

| Capability                   | Contract                                                                 |
| ---------------------------- | ------------------------------------------------------------------------ |
| `server.game-integration.v1` | State and event publication transport                                    |
| `server.game-events.v1`      | Persistent event queues and panel acknowledgements after database commit |
| `server.game-media.v1`       | Separate authenticated media uploads                                     |

A connected panel without durable event support causes event publication to
return `503`. Media also returns `503` when its support was not negotiated. A
successful socket connection alone does not establish that every publication
type is supported.

Start or restart the game server through Campfire after enabling the feature.
Newly created game containers receive:

- A read-only bind mount at `/run/campfire-game` containing `publish.sock`.
- `CAMPFIRE_GAME_SOCKET=/run/campfire-game/publish.sock` in their environment.

The directory mount is read-only; connecting to the socket and publishing through
its HTTP API is still allowed.

Existing running containers need recreation through a server restart to gain the
mount. Spark recreates listeners for saved server configurations after its own
restart; directory mounts allow containers to reconnect to the replacement socket.
Disabling publishing revokes access when Spark receives the updated configuration
and removes the listener. Restart the game server to remove its container mount.

Keep Spark's data directory private and on the same host filesystem visible to
Docker, as with existing game-data mounts. Use a short absolute `--data` path:
Unix socket paths are OS-limited (typically 107 bytes on Linux), and startup fails
if an endpoint path is too long. Endpoint subdirectories use hashed server IDs.

Each game sees only its own socket directory. Socket permissions accommodate
arbitrary game UIDs; the private host parent and isolated read-only container
mount provide access control. No network listener, public port, game API token,
or Spark identity credential is exposed. Host administrators can access sockets.
Every request also checks the saved local configuration, and Campfire checks
authoritative placement, rejecting deleted or reassigned servers.

## JSON envelope and payload schemas

Send `POST /v1/publish` over the Unix socket with a version 1 JSON object containing
exactly one `state` or `event`. Do not include a server ID: the socket determines it.

| Property  | Required            | Type and meaning                       |
| --------- | ------------------- | -------------------------------------- |
| `version` | Yes                 | Integer `1`                            |
| `state`   | Exactly one payload | Complete state object, described below |
| `event`   | Exactly one payload | Event object, described below          |

Do not batch multiple envelopes, send newline-delimited JSON, or include both
payloads in one request. The raw JSON request must fit within 16 KiB, including
whitespace. Unknown envelope, state, and event properties are rejected. Put custom
values inside `state.fields` or `event.data`.

### State snapshots

```sh
curl --unix-socket "$CAMPFIRE_GAME_SOCKET" \
  --include --max-time 12 \
  -H 'Content-Type: application/json' \
  --data '{"version":1,"state":{"players":3,"capacity":20,"fields":{"game_mode":"survival"}}}' \
  http://spark/v1/publish
```

State is a complete replacement snapshot. Omitted fields clear previous values
in the dashboard. `players` and `capacity` are optional integers
between 0 and 1,000,000. `fields` is an optional map for game-specific scalar
values, such as `map` or `game_mode`. An empty `state` object is valid.

| State property | Required | Type and limits               |
| -------------- | -------- | ----------------------------- |
| `players`      | No       | Integer from `0` to `1000000` |
| `capacity`     | No       | Integer from `0` to `1000000` |
| `fields`       | No       | Up to 32 named scalar values  |

For example, `{"version":1,"state":{}}` clears the previously published snapshot.
An omitted player count means unknown; use `0` only when you know there are no
players. State has no producer timestamp or event ID. Campfire records when it
receives the snapshot.

### Events

Save the following object as `event.json` for the request below. In a real mod,
generate the event ID and occurrence timestamp once when the event happens.

```json
{
  "version": 1,
  "event": {
    "id": "announcement-unique-id",
    "type": "game.announcement",
    "occurred_at": "2026-10-24T00:00:00Z",
    "title": "A new adventure begins",
    "data": { "world": "survival" }
  }
}
```

```sh
curl --unix-socket "$CAMPFIRE_GAME_SOCKET" \
  --include --max-time 12 \
  -H 'Content-Type: application/json' \
  --data-binary @event.json \
  http://spark/v1/publish
```

| Event property | Required | Type and limits                                                   |
| -------------- | -------- | ----------------------------------------------------------------- |
| `id`           | Yes      | String, 1–128 bytes; unique within this server's retained history |
| `type`         | Yes      | Name matching the pattern below; namespacing is recommended       |
| `occurred_at`  | Yes      | Nonzero RFC 3339 timestamp; UTC with `Z` is recommended           |
| `title`        | Yes      | String, 1–256 bytes                                               |
| `data`         | No       | Up to 32 named scalar values                                      |

Event IDs must stay the same across retries. Campfire deduplicates them per server
within retained history; the first stored payload wins. Reusing a queued ID with
different content returns HTTP 409. Use a new ID for each distinct event.

A UUID string is a suitable ID. Keep the entire envelope unchanged across retries,
including `occurred_at`, `title`, and `data`; generating a new timestamp during a
retry changes the queued event. Two different servers may use the same event ID.
The ID is not a global identity or an authorization token.

Event types are supplied by the integration. Examples include `player.joined`,
`player.left`, and `game.announcement`; game-specific names such as
`valheim.boss_defeated` are also valid. Campfire does not require rounds, matches,
or any particular game model, and it does not translate an event type into a game
command.

### Custom scalar fields

Field names and event types start with a lowercase ASCII letter and contain only
lowercase letters, digits, underscores, dots, or hyphens, up to 64 bytes. Each
scalar map permits at most 32 entries whose values are strings (up to 1,024 bytes),
finite JSON numbers, or booleans. Nested objects, arrays, and null values are
unsupported in these maps.

The name pattern is `^[a-z][a-z0-9_.-]{0,63}$`. Length limits are byte limits,
not counts of displayed characters. Use strings for opaque IDs and exact large
integers; JSON numbers are decoded as floating-point values. For collections,
publish summaries or separate events rather than embedding arrays or objects.

Valid scalar fields look like:

```json
{
  "map": "world",
  "pvp": true,
  "day": 42
}
```

This map belongs inside `state.fields` or `event.data`; it is not a publication
envelope by itself.

## Current state and freshness

All game fields are optional. Games without player counts, maps, game modes, or
other fields show none of those values. A capacity without a player count is shown
as capacity; a missing player count is never interpreted as zero. Custom scalar
fields are displayed as text, numbers, or booleans. An empty snapshot clears the
previous fields.

Publish a complete snapshot periodically, for example every 15 seconds, even if
the values have not changed. Snapshots become stale after 60 seconds without an
update. The dashboard shows the last update time and replaces stale values with
"Game data unavailable." The browser also expires displayed values if monitoring
requests stop succeeding. Events do not refresh the state timestamp.

Game-data freshness is independent of runtime status. A server can remain running
while its game data is unavailable. Unknown or disconnected runtime observations
also suppress game values until the runtime is confirmed running again.

Lifecycle changes such as stop, restart, reinstall, or restore invalidate current
game state immediately. State publications are temporarily rejected during the
transition until a subsequent healthy, non-busy running observation reopens
ingestion. Producers should retry with bounded backoff. The existing runtime
sampler also detects stops and changes in Docker start times for restarts performed
outside Campfire. External changes take effect on the next runtime observation.

Game state is returned as the optional `game` member of each authorized server in
`GET /api/v1/monitoring`. The object includes `updated_at`, `fresh_for_seconds`, and
`status` (`fresh`, `stale`, or `unavailable`). Only fresh, runtime-valid responses
include `state`. Servers that have never published state omit `game`. Existing
server visibility and token permissions apply to the monitoring response.

## Event history and delivery

Spark persists events in private, atomically written queue files before returning
HTTP 202. This means queued locally, not yet delivered. The queue survives Spark
restarts. Campfire acknowledges an event only after its database transaction
commits; Spark then removes the queue entry. Retries after a lost acknowledgement
use the same event ID, so they do not create duplicate retained entries.

The queue holds at most 512 events per server and 4,096 per Spark, with a 24-hour
delivery lifetime. When full, it rejects new events with HTTP 429 and
`Retry-After: 5`, preserving existing entries. Transient delivery failures retry
with backoff up to 30 seconds; reconnecting triggers an immediate retry.
Expired events and events permanently rejected because their server was deleted
or reassigned are dropped with a warning in Spark's log. This bounded queue does
not promise delivery after its lifetime expires or server placement changes.

Inspect a server's queue over its own socket:

```sh
curl --unix-socket "$CAMPFIRE_GAME_SOCKET" http://spark/v1/events/queue
```

The response contains `pending`, `limit`, `node_limit`, `dropped`, and `last_error`.
Failure counters are local to this Spark process; pending queue files persist
across restarts. A corrupt or oversized spool file prevents queue startup rather
than silently deleting accepted data.

A queue response looks like:

```json
{
  "pending": 1,
  "limit": 512,
  "node_limit": 4096,
  "dropped": 0,
  "last_error": ""
}
```

This is aggregate status for the server, not a receipt for an individual event.
`pending: 0` can mean delivery completed or events were dropped. Use retained event
history to verify a particular event end to end.

Events are attempted oldest first, but a delayed retry can arrive after newer
events. Do not depend on strict delivery order. Store a game-specific sequence in
`data` when consumers need it. The queue can retry an event more than once;
per-server deduplication prevents additional retained entries while that event's
ID remains in history. This is not an unlimited or exactly-once delivery promise.

Campfire retains the newest 10,000 events per server for up to 30 days by default.
Retention uses the trusted receipt time, independently of the game's clock.
Limits apply during ingestion and cleanup runs at startup and hourly, including
quiet servers. Configure them with:

| Flag                          | Environment variable                 | Default | Range    |
| ----------------------------- | ------------------------------------ | ------- | -------- |
| `--game-event-retention-days` | `CAMPFIRE_GAME_EVENT_RETENTION_DAYS` | 30      | 1–3650   |
| `--game-event-limit`          | `CAMPFIRE_GAME_EVENT_LIMIT`          | 10000   | 1–100000 |

Once an event expires or is removed by the count limit, its ID is no longer in the
deduplication window. Deleting a server also deletes its retained events.

The authorized endpoint `GET /api/v1/servers/{id}/events` returns
`{ "items": [...], "next_before": 123 }`. `next_before` is omitted at the end.
Supported query parameters are `limit` (1–100, default 50), `before` (receipt-order
ID cursor), `type` (exact namespaced type), and `from`/`to` (Unix seconds filtering
occurrence time, inclusive start and exclusive end). Each event includes its
original `event_id`, type, title, occurrence and receipt times, source server and
Spark IDs, and scalar data. Ordering follows receipt IDs, so clock differences
cannot cause cursor pagination to skip events. Existing `server.view` permissions
and token scopes protect access; operating the server is not required.

The Game activity tab offers type/time filters, older-page navigation, and a
return to newest events. The newest page refreshes every ten seconds. It presents
game-specific event types as supplied, without assuming matches or rounds.

## Screenshots and clips

Media travels separately from JSON publications and the control websocket.
Publish an event first, then send the raw file to the same mounted socket:

```sh
curl --unix-socket "$CAMPFIRE_GAME_SOCKET" \
  --request PUT \
  --include --max-time 125 \
  --header 'Content-Type: image/png' \
  --header 'X-Campfire-Media-Name: world.png' \
  --data-binary @world.png \
  http://spark/v1/events/announcement-unique-id/media/screenshot-1
```

The event segment is the original producer event ID (`announcement-unique-id` in
the JSON example), URL-encoded as a path segment. Use a URL encoder for each path
segment, rather than encoding the entire path or concatenating unescaped IDs.
The final segment is a stable attachment ID containing 1–64 ASCII letters,
digits, underscores, or hyphens. File names are 1–128 UTF-8 bytes without path
separators or control characters. The MIME type must be exactly `image/png`,
`image/jpeg`, `video/mp4`, or `video/webm` and match the file signature. Screenshot
headers must describe a valid image no larger than 40 million pixels. Clips are
stored as supplied without transcoding or server-side capture; playback depends
on the browser's codec support.

Send the file as the request body, not multipart form data, base64, or a JSON
field. Set `X-Campfire-Media-Name` to the display filename and `Content-Type` to
the exact MIME type without parameters. Supply `Content-Length` when your client
knows it; streamed uploads are still subject to the byte limit.

Spark derives server identity from the socket, then streams the upload over HTTPS
to Campfire's dedicated agent listener using its existing client certificate and
trust configuration. There is no new game credential or public upload endpoint.
Both peers must negotiate `server.game-media.v1`. Uploads have a two-minute
deadline and two concurrent upload slots per Spark and across Campfire.

| Limit                          | Value   |
| ------------------------------ | ------- |
| Single attachment              | 25 MiB  |
| Attachments per event          | 4       |
| Retained media per server      | 250 MiB |
| Retained media across Campfire | 1 GiB   |

These are fixed initial limits. Quota checks and immutable attachment identity
commit in the same database transaction. Metadata and bounded file bytes are
stored together in SQLite or PostgreSQL. The media budgets bound logical stored
bytes; database file/WAL space and backups can be larger. Event list responses
include attachment metadata without loading or returning file bytes.

Unlike events, media is not queued on Spark. `204` confirms a committed upload;
retrying the same attachment ID with identical name, MIME type, and bytes succeeds
without adding another attachment. A lost response has an uncertain outcome, so
reuse the same ID and bytes. Changed content or metadata returns `409`.

An event's local `202` acknowledgement may precede its arrival at Campfire. Media
for an event not yet retained returns `409` with `Retry-After: 5`; retry after the
event queue catches up. `429` means upload slots are busy; `503` means disconnected,
unsupported, or temporarily unavailable. Respect `Retry-After`, use bounded
backoff, and retain the original file until upload succeeds or the producer gives
up. `400` rejects invalid metadata/content, `403` rejects source placement, and
`413` rejects file, attachment-count, or storage limits. No partial attachment is
visible after failed ingestion. Do not retry permanent rejections indefinitely.

An attachment's lifetime follows its parent event. Age/count pruning and server
deletion cascade to media in the same database. Deletion frees the logical quota;
it does not promise immediate shrinking of database files on disk.

The activity feed displays lazy-loaded images, clips with playback controls and
no autoplay, and download links. `GET /api/v1/servers/{id}/events/{event}/media/{attachment}`
uses the panel's numeric event ID and requires `server.view` authorization,
including token scopes. It supports HTTP ranges for seeking; `?download=1`
requests a download. Responses are private with no caching and cannot be used as
public media links. Forwarding events or media to Discord is not implemented.

Keep these identifiers separate when building readers or attachment uploaders:

| Identifier          | Assigned by       | Used for                                                                      |
| ------------------- | ----------------- | ----------------------------------------------------------------------------- |
| Producer event `id` | Your integration  | Deduplication and the local media-upload URL                                  |
| Panel event `id`    | Campfire database | Reading retained media through `/api/v1/servers/.../events/{event}/media/...` |
| Attachment `id`     | Your integration  | One immutable attachment within the event, for both upload and reading        |

The panel event response exposes the producer ID as `event_id`. Do not substitute
the numeric panel ID into the local socket's upload path.

## Request limits

- JSON publication requests are limited to 16 KiB; media uses the separate limits above.
- Each server permits 10 JSON publication requests per one-second window. Media
  uses its separate upload slots; queue inspection is a separate GET route.
- Spark permits 32 in-flight publications across its local endpoints.
- State publications wait up to eight seconds for acknowledgement. Events use the
  durable queue; a local timeout can have an uncertain outcome, so retry with the
  same event ID and payload.
- Only the latest state is held in memory; no game-state history is stored.

| HTTP status | Meaning                                                                             |
| ----------- | ----------------------------------------------------------------------------------- |
| 204         | Campfire validated and acknowledged the publication                                 |
| 202         | Spark durably queued the event for delivery                                         |
| 400         | Invalid schema, version, or trailing JSON                                           |
| 403         | Local server configuration no longer authorizes this endpoint                       |
| 409         | A pending event has the same ID but different content                               |
| 413         | Request exceeds the size limit                                                      |
| 429         | Request rate exceeded (one second) or event queue full (five seconds)               |
| 503         | Busy, disconnected, unsupported, rejected by Campfire, or acknowledgement timed out |

## Client lifecycle, retries, and backpressure

Capture game state through the game's supported API or hooks. If those APIs must
run on the game thread, take a snapshot there and hand it to a background worker.
Do not perform socket I/O, media uploads, sleeps, or retries on the simulation
thread. Serialization and transport should not stall a tick or player action.

A typical integration uses the following lifecycle:

1. Read `CAMPFIRE_GAME_SOCKET` and start a background publisher when configured.
2. Send a complete state snapshot after startup, then periodically, such as every
   15 seconds. Keep only the newest unsent snapshot while disconnected.
3. On each event hook, create one envelope with a new ID and fixed occurrence
   time. If loss before Spark accepts it matters, persist it in a bounded
   producer outbox before sending.
4. Retry transient failures using the same event envelope. After `202`, Spark
   owns the queued copy; the producer can remove its own outbox copy. This does
   not guarantee later panel delivery beyond Spark's queue lifetime and placement
   rules.
5. For attachments, retain the original bytes and metadata until upload returns
   `204`. Retry the same attachment ID with exactly the same content.
6. During shutdown, use a short, bounded flush period for any final event. Do not
   block server shutdown indefinitely or invent a player count when it is unknown.

Use capped exponential backoff with jitter and a finite retry budget. Respect
`Retry-After` when present, and rate-limit normal traffic so event bursts do not
consume all publication requests needed for state heartbeats.

| Outcome                                      | Producer action                                                                                                                                                      |
| -------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Missing socket, connection reset, or timeout | Reconnect on a later attempt. An event may already have been accepted, so reuse its ID and payload. For state, send the newest complete snapshot.                    |
| `202` for an event                           | Stop resubmitting; Spark accepted responsibility for its bounded queue.                                                                                              |
| `204` for state or media                     | The corresponding panel operation succeeded.                                                                                                                         |
| `400` or `413`                               | Correct the schema, metadata, encoding, or size. For media, also check attachment-count and retained-storage quotas. Do not retry an over-limit upload indefinitely. |
| `401` on media upload                        | Spark's panel authentication failed. Check the agent connection and credentials; adding a game API token will not fix it.                                            |
| `403`                                        | Stop publishing through this endpoint and report that local configuration or server placement no longer permits it.                                                  |
| `404` or `405`                               | Check the HTTP method and local `/v1` route; these are not panel `/api/v1` write endpoints.                                                                          |
| `409` for a JSON event                       | A pending event ID was reused with different content. Restore the original payload or use a new ID for a distinct event.                                             |
| `409` for media with `Retry-After`           | The parent event has not reached retained history yet. Retry the unchanged attachment later.                                                                         |
| `409` for media without `Retry-After`        | The attachment ID already has different content or metadata; do not overwrite it.                                                                                    |
| `429`                                        | Apply backpressure. Wait for `Retry-After` before trying again; keep queues bounded.                                                                                 |
| `503`                                        | Retry with bounded backoff. If persistent, check connection, feature compatibility, Spark logs, and runtime transitions.                                             |

Success responses have no JSON body. Log the HTTP status and bounded response text
for failures. If a mod does not support media, it can implement state and event
publishing without any attachment handling.

### Minimal Node.js transport

This example uses only Node.js built-ins and makes one event-publication attempt.
It demonstrates the socket transport and response handling; a production mod's
worker should implement the retry and outbox policy above. In particular, retain
the constructed `publication` instead of generating a new ID on each attempt.

```js
import http from "node:http";
import { randomUUID } from "node:crypto";

const socketPath = process.env.CAMPFIRE_GAME_SOCKET;

async function publish(publication) {
  if (!socketPath) throw new Error("Campfire integration is not configured");
  const body = Buffer.from(JSON.stringify(publication));
  if (body.length > 16 * 1024) throw new Error("Publication exceeds 16 KiB");

  return new Promise((resolve, reject) => {
    const request = http.request(
      {
        socketPath,
        hostname: "spark",
        path: "/v1/publish",
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Content-Length": body.length,
        },
        timeout: 12000,
      },
      (response) => {
        let detail = "";
        response.setEncoding("utf8");
        response.on("data", (chunk) => {
          detail += chunk;
        });
        response.on("error", reject);
        response.on("end", () =>
          resolve({
            status: response.statusCode,
            retryAfter: response.headers["retry-after"],
            detail,
          }),
        );
      },
    );
    request.on("timeout", () =>
      request.destroy(new Error("Publication timed out")),
    );
    request.on("error", reject);
    request.end(body);
  });
}

const publication = {
  version: 1,
  event: {
    id: randomUUID(),
    type: "game.announcement",
    occurred_at: new Date().toISOString(),
    title: "Integration started",
    data: { mod_version: "1.0.0" },
  },
};

const result = await publish(publication);
if (result.status !== 202) {
  throw new Error(`HTTP ${result.status}: ${result.detail}`);
}
console.log("Spark queued the event; panel delivery may still be pending.");
```

For this demonstration, save the code as `publisher.mjs` and run it inside the
opted-in game container with `node publisher.mjs`. Game runtimes without Node.js
can send the same HTTP requests with their own Unix-socket-capable client.

## Testing and troubleshooting an integration

Start with a real Spark and one opted-in game server. Use the curl requests above
from inside its container, then check the server's current state and Game activity
in Campfire. Test your mod with these cases before relying on its delivery:

- Clear state with an empty snapshot, and omit a previously supplied custom field.
- Resend an event with the same ID and payload; retained history should contain
  one entry within the deduplication window.
- Disconnect the panel: state requests should fail transiently, while supported
  events can queue. Reconnect and check that retained history catches up.
- Restart Spark with events pending and verify they survive. Reconnect any stale
  HTTP connection to the unchanged socket path.
- Let state heartbeats stop for more than 60 seconds and confirm stale values are
  no longer presented as current.
- Upload an attachment immediately after an event's `202`, handle the possible
  `409`, then retry the identical upload after the event arrives.
- Exercise shutdown and failed publication without blocking the game thread.

| Symptom                                           | Check                                                                                                                    |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Environment variable absent                       | Integration is enabled for this server and its container was recreated through Campfire.                                 |
| Socket missing or connection refused              | Correct container and socket path; Spark is running; reconnect after a restart.                                          |
| `400` on otherwise valid-looking JSON             | Exactly one version 1 envelope, no extra properties, valid timestamps, scalar-only maps, and byte lengths within limits. |
| State returns `503` around restart or restore     | Wait for a healthy running observation, then send the newest complete snapshot.                                          |
| Events return `202` but do not appear             | Inspect queue status, Spark logs, panel connectivity, event capability support, expiration, and placement changes.       |
| Player counts disappear while the server runs     | Send periodic full snapshots; event traffic does not refresh state freshness.                                            |
| Media returns `400`                               | Exact MIME header, matching file signature, valid filename, raw bytes, and supported image dimensions.                   |
| Media repeatedly returns `409` with `Retry-After` | Verify the producer event ID, queue delivery, and that the parent event remains in retained history.                     |

Spark service logs are available through `journalctl -u spark`; panel ingestion
errors are in the Campfire logs. Do not add panel administrator tokens or Spark
private keys to a mod to work around socket or placement errors.

## Example publisher

For dashboard and destination development without a game server, `make dev-mock`
runs a live simulated publisher. See the
[mock game integration guide](/Campfire/development/#mock-game-integration) for
scenarios and console controls. It exercises panel behavior without providing a
real game mod.

The repository also includes a [small command-line publisher](https://github.com/dougmaitelli/Campfire/blob/master/scripts/game-publisher.mjs).
Copy `scripts/game-publisher.mjs` into a game container with Node.js installed:

```sh
node game-publisher.mjs state
node game-publisher.mjs event
```

The script discovers the socket through `CAMPFIRE_GAME_SOCKET`. For host-side
development, pass the specific server's socket path as its second argument.
It deliberately performs one publication without automatic retries.

## Protocol implementation references

When implementing a client, the contracts are defined by these sources:

- [Publication schemas and limits](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/shared/model/game_integration.go)
- [Local HTTP routes and forwarding](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/spark/agent/game_integration.go)
- [Persistent event queue and retry behavior](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/spark/agent/game_event_queue.go)
- [Media metadata and limits](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/shared/model/game_media.go)
- [Media forwarding and authentication](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/spark/agent/game_media.go)
- [Panel placement checks and state freshness](https://github.com/dougmaitelli/Campfire/blob/master/backend/internal/campfire/service/game_integration.go)
