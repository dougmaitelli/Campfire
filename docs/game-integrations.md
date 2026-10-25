# Game integrations

Game mods and adapters can publish state snapshots and events to a local Spark
Unix socket. Spark forwards them over its existing authenticated connection.
Campfire validates each publication against the server's current Spark placement
and acknowledges it.

State publications now appear in the overview, server list, and server operations
page. Campfire keeps only the latest snapshot per server in memory. There are no
game-state charts or history, and a Campfire restart clears these snapshots.
Events are durably retained in the database and appear in each server's
**Game activity** tab. Discord delivery and media attachments are later stages.

## Enable publishing

Start Spark with `--game-integrations` or `SPARK_GAME_INTEGRATIONS=true`. The feature
is disabled by default and advertised as `server.game-integration.v1` only when
enabled. Durable event delivery additionally negotiates `server.game-events.v1`.
An older Campfire can still receive supported state snapshots, but Spark will not
send queued events through the older, non-durable ingestion contract.

Start or restart the game server through Campfire after enabling the feature.
Newly created game containers receive:

- A read-only bind mount at `/run/campfire-game` containing `publish.sock`.
- `CAMPFIRE_GAME_SOCKET=/run/campfire-game/publish.sock` in their environment.

Existing running containers need recreation through a server restart to gain the
mount. Spark recreates listeners for saved server configurations after its own
restart; directory mounts allow containers to reconnect to the replacement socket.
Disabling the feature removes listeners when Spark exits; recreate game containers
to remove their mounts as well.

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

## Publish

Send `POST /v1/publish` over the Unix socket with a version 1 JSON object containing
exactly one `state` or `event`. Do not include a server ID: the socket determines it.

```sh
curl --unix-socket "$CAMPFIRE_GAME_SOCKET" \
  -H 'Content-Type: application/json' \
  --data '{"version":1,"state":{"players":3,"capacity":20,"fields":{"game_mode":"survival"}}}' \
  http://spark/v1/publish
```

State is a complete replacement snapshot. Omitted fields clear previous values
in the dashboard. `players` and `capacity` are optional integers
between 0 and 1,000,000. `fields` is an optional map for game-specific scalar
values, such as `map` or `game_mode`. An empty `state` object is valid.

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

Events require an ID of 1–128 bytes, a type, an RFC 3339 timestamp, and a title of
1–256 bytes. `data` is an optional scalar map. Namespaced types are supported.
Event IDs must stay the same across retries. Campfire deduplicates them per server
within retained history; the first stored payload wins. Reusing a queued ID with
different content returns HTTP 409. Use a new ID for each distinct event.

Field names and event types start with a lowercase ASCII letter and contain only
lowercase letters, digits, underscores, dots, or hyphens, up to 64 bytes. Each
scalar map permits at most 32 entries whose values are strings (up to 1,024 bytes),
finite JSON numbers, or booleans. Nested objects, arrays, and null values are
unsupported in these maps. Unknown envelope/state/event properties are rejected.

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

## Request limits

- Requests are limited to 16 KiB.
- Each server endpoint permits 10 requests per one-second window.
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

## Example publisher

For dashboard and destination development without a game server, `make dev-mock`
runs a live simulated publisher. See the
[mock game integration guide](development.md#mock-game-integration) for scenarios
and console controls. A real Valheim integration will live in a separate repo.

Copy `scripts/game-publisher.mjs` into a game container with Node.js installed:

```sh
node game-publisher.mjs state
node game-publisher.mjs event
```

The script discovers the socket through `CAMPFIRE_GAME_SOCKET`. For host-side
development, pass the specific server's socket path as its second argument.
It deliberately performs one publication without automatic retries.

See [the implementation plan](game-integrations-plan.md) for later stages.
