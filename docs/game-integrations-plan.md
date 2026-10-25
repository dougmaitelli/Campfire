# Game integrations implementation plan

Status: stages 1–4 and 6 implemented; Discord stages 5 and 7 deferred until last. See
[the transport documentation](game-integrations.md) for current functionality.

## Objective and scope

Allow game mods, plugins, and adapters to publish current game state and events to
Campfire through Spark. Use the same information in the dashboard and external
destinations such as Discord.

- Games publish to Spark locally. Spark forwards data over its existing
  authenticated connection to Campfire.
- Local publishers must be bound to their own server. They never receive Spark's
  Campfire credentials.
- Keep only the latest game state. Do not add state history or charts.
- Do not include tick rate in the initial scope.
- Display fields only when the game integration supplies them. Maps, game modes,
  matches, and similar concepts are optional.
- Retain event history separately from current state.
- Keep Discord credentials and routing configuration in Campfire.
- Accept media produced by game tooling; capture and recording depend on the game.

```text
Game mod or adapter
        |
        | Local endpoint
        v
      Spark
        |
        | Existing authenticated connection
        v
     Campfire
        |-- Current state --> Dashboard
        |-- Events --------> Event history and activity feed
        `-- Routing -------> Discord and future destinations
```

## Stage 1: Local ingestion and transport

Implemented with per-server Unix sockets, isolated read-only container mounts,
optional capability negotiation, bounded publications, and panel acknowledgements.
Stage 1 provides transport only; stage 2 adds the current-state projection.
Stage 3 adds event persistence; external delivery is not included yet.

Establish the path from a game server to Campfire.

- Expose an opt-in Spark endpoint reachable from the game container.
- Bind each publisher to its server through an isolated socket mount. No separate
  game-to-Spark credential is needed, and publishers cannot supply their server ID.
- Extend the Spark protocol with an optional game-integration capability.
- Define versioned messages for state snapshots and events.
- Apply payload limits, rate limits, and validation at Spark.
- Provide a small example publisher for development.
- Make state snapshots replace the previous snapshot so omitted fields disappear.
- Give events unique IDs for deduplication.

Completion criteria: an example publisher sends state and an event through Spark,
and Campfire attributes both to the correct server. Sparks without the capability
continue working.

## Stage 2: Current state in the dashboard

Implemented with replacement snapshots held in memory, optional fields in the
existing authorized monitoring response, and readings on overview, server list,
and server operations pages. Freshness expires after 60 seconds; lifecycle changes
and runtime sampling invalidate old values. No game-state history or charts are
created.

Expose useful game information without historical storage.

- Store only the latest state per server.
- Include optional standard fields such as player count and capacity.
- Support optional map, game mode, and other bounded, typed fields.
- Display only supplied fields. Games without maps or matches show neither.
- Show the last update time and mark state stale after a defined timeout.
- Invalidate state when the game stops or restarts.
- Keep runtime status and game-data freshness separate.
- Do not create charts or state history.

For example, one server might show `8 / 20 players · Survival`, while another
shows only `3 players`.

Completion criteria: dashboard details reflect incoming snapshots, removed fields
disappear, and stale or stopped servers no longer present old values as current.

## Stage 3: Events and event history

Implemented with transactional per-server deduplication, database-backed history,
configurable age/count limits, an authorized cursor API, and a Game activity tab.
Spark queues events on disk with capacity limits, bounded retries and a 24-hour
lifetime. A separate negotiated capability requires durable panel acknowledgements.

Add durable game activity independently of current state.

- Define an event envelope containing an ID, type, timestamp, title, structured
  data, and source.
- Offer optional standard types such as player joined, player left, and
  announcement.
- Allow namespaced game-specific types such as `minecraft.advancement`.
- Persist events with configurable retention and bounded storage.
- Add a server activity feed with basic filtering.
- Deduplicate retried events.
- Define a bounded Spark queue and explicit behavior while Campfire is disconnected.
- Support matches, rounds, and achievements only where an integration emits them.

Completion criteria: events survive a Campfire restart, retries do not create
duplicate entries, and retention works.

## Stage 4: Simulated game integration in dev-mock

Implemented with five-second state heartbeats, a fifteen-second event cycle,
mock-only console controls for pause/resume/advance/replay, and functional tests
against the real ingestion services and database. See the
[development guide](development.md#mock-game-integration) for usage.

Exercise the game-data flows during development before building a real game mod.
Keep the simulator in the existing dev-mock package and cover its behavior with
the normal functional and end-to-end test suites.

- Simulate changing player counts, joins, leaves, and announcements for a running
  mock server, rather than relying only on seeded event history.
- Publish through the existing game ingestion services so validation, current
  state, durable events, and later destination routing use the application paths.
- Supply only relevant optional fields; omit map and match concepts when absent.
- Respect start, stop, and restart behavior, including state freshness.
- Provide deterministic scenarios for development and automated tests, including
  duplicate event delivery and unavailable or stale state.
- Reuse these scenarios when implementing Discord publishing and media delivery.

Completion criteria: dev-mock demonstrates changing dashboard state and new
durable events without Docker or a game installation. This does not validate the
real game-to-Spark Unix socket transport; that remains covered by transport tests
and must also be checked with the first real integration.

### First real integration: Valheim (deferred)

Build the Valheim mod or adapter in a separate repository after the end-to-end
product flows are ready. Campfire owns the shared contract and its documentation;
the integration repository owns game-specific code, packaging, and installation.

- Publish available player information and selected events through Spark's
  mounted Unix socket, without direct access to Campfire or separate credentials.
- Document installation, socket discovery, mount isolation, and reconnection.
- Validate against a real hosted Valheim server and adjust the shared contract
  based on that experience.

The mock integration lets stages 5 and 6 proceed independently; real Valheim
compatibility remains a separate validation milestone.

## Stage 5: Discord publishing

Deferred until after stage 6. This stage will also publish retained screenshots
and clips as Discord attachments or links.

Introduce external destinations and routing.

- Configure Discord destinations in Campfire and keep credentials there.
- Route selected event types from selected servers to a destination.
- Provide message templates and a test-delivery action.
- Optionally maintain one status message per server using its latest available
  state.
- Throttle status updates and mark stale data appropriately.
- Queue event deliveries with bounded retries and visible failures.
- Disable unsolicited mentions by default.
- Retain the existing Apprise path for general operational notifications; share
  delivery infrastructure where practical.

Example rules:

| Trigger                            | Action                                             |
| ---------------------------------- | -------------------------------------------------- |
| First player joins an empty server | Announce that the server is active                 |
| Player count changes               | Update a persistent status message with throttling |
| Match ends, where supported        | Post the result                                    |
| Game emits an announcement         | Publish it to the configured channel               |
| Game data becomes stale            | Mark the status message's game data unavailable    |

Completion criteria: game events produce Discord messages, and an optional status
message reflects current player information without channel spam.

## Stage 6: Screenshots and clips

Implemented ahead of Discord with a bounded Unix-socket upload relayed over
Spark's existing mTLS identity, immutable event attachments in the database,
storage quotas, cascading event retention, authorized range downloads, and media
in the Game activity feed. Dev-mock supplies synthetic screenshots and clips.

Extend events with media attachments.

- Accept media supplied by a mod, adapter, spectator client, or recording tool.
- Transfer media through a bounded upload path via Spark, avoiding large payloads
  on the control connection.
- Associate attachments with retained events.
- Apply size limits, storage quotas, and retention.
- Leave Discord attachments or links to stage 5, which now comes later.
- Show media in the dashboard event feed.

This stage handles media delivery. A headless game server may require separate
client or recording tooling to produce screenshots and clips.

Completion criteria: a publisher submits an image or clip through Spark that
appears in its retained event and can be viewed or downloaded by authorized
users. Automated transport tests and dev-mock validate this path; validation
against the real Valheim integration remains deferred with that integration.

## Stage 7: Interactive Discord features (optional)

- Add `/status` and `/players` using the existing current-state model.
- Treat server controls and bidirectional chat as separate follow-up work.
- Define explicit permissions and account mapping before enabling administrative
  actions.

Completion criteria: authorized Discord users can query available server state,
and unavailable or stale fields are represented accurately.

## Suggested implementation increments

1. Stages 1 and 2: transport and current-state dashboard support.
2. Stage 3: event history.
3. Stage 4: simulated game integration in dev-mock.
4. Stage 6: media delivery.
5. Stage 5: Discord publishing, including media.
6. Stage 7: optional interactive Discord features.

Develop the first real integration, Valheim, in a separate repository once the
dashboard and external delivery flows are ready.

Stage 1 uses Unix sockets and per-container mount isolation for local identity.
Stage 2 uses a 60-second freshness timeout. Stage 3 defines the durable event
queue and configurable event retention. Stage 6 defines media limits and
retention in the [transport documentation](game-integrations.md#screenshots-and-clips).
