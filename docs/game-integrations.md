# Game publishing transport (stage 1)

Game mods and adapters can publish state snapshots and events to a local Spark
Unix socket. Spark forwards them over its existing authenticated connection.
Campfire validates each publication against the server's current Spark placement
and acknowledges it.

This is the ingestion foundation. Publications do not yet appear in the dashboard,
persist in event history, or reach Discord. An HTTP 204 confirms validated
ingestion only. With debug logging enabled, Campfire logs accepted publication
metadata without game payload contents.

## Enable publishing

Start Spark with `--game-integrations` or `SPARK_GAME_INTEGRATIONS=true`. The feature
is disabled by default and advertised as `server.game-integration.v1` only when
enabled. Both Spark and Campfire must support this capability; an older Campfire
leaves publishing unavailable without breaking ordinary server operations.

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
when a state consumer is added. `players` and `capacity` are optional integers
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
Event IDs must stay the same across retries. Durable deduplication and event
retention will be implemented in stage 3; they are not provided by this transport.

Field names and event types start with a lowercase ASCII letter and contain only
lowercase letters, digits, underscores, dots, or hyphens, up to 64 bytes. Each
scalar map permits at most 32 entries whose values are strings (up to 1,024 bytes),
finite JSON numbers, or booleans. Nested objects, arrays, and null values are
unsupported in these maps. Unknown envelope/state/event properties are rejected.

## Limits and delivery

- Requests are limited to 16 KiB.
- Each server endpoint permits 10 requests per one-second window.
- Spark permits 32 in-flight publications across its local endpoints.
- Spark waits up to eight seconds for acknowledgement after decoding a request.
- There is no offline queue in stage 1. Producers should retry with bounded
  backoff and reuse event IDs. An acknowledgement timeout has an uncertain outcome:
  Campfire may have accepted the publication before the connection failed.
- No history or latest-state projection is stored in stage 1.

| HTTP status | Meaning                                                                             |
| ----------- | ----------------------------------------------------------------------------------- |
| 204         | Campfire validated and acknowledged the publication                                 |
| 400         | Invalid schema, version, or trailing JSON                                           |
| 403         | Local server configuration no longer authorizes this endpoint                       |
| 413         | Request exceeds the size limit                                                      |
| 429         | Server endpoint rate limit; retry after one second                                  |
| 503         | Busy, disconnected, unsupported, rejected by Campfire, or acknowledgement timed out |

## Example publisher

Copy `scripts/game-publisher.mjs` into a game container with Node.js installed:

```sh
node game-publisher.mjs state
node game-publisher.mjs event
```

The script discovers the socket through `CAMPFIRE_GAME_SOCKET`. For host-side
development, pass the specific server's socket path as its second argument.
It deliberately performs one publication without automatic retries.

See [the implementation plan](game-integrations-plan.md) for later stages.
