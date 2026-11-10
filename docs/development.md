---
title: Development
---

Use Go 1.26 or newer, a C compiler for SQLite/CGO, Node.js 26, pnpm 11.28.5, and Make. Docker is needed for real game execution and the canonical browser test environment.

```sh
make dependencies
make dev-mock
```

Open `http://127.0.0.1:8081` and sign in with `admin` / `campfire-dev-password`. Mock mode runs the real authentication, API, database, and services against simulated Sparks. It includes sample servers, roles, notifications, files, console output, backups, and transfers without needing Docker or production credentials. Each new `make dev-mock` session resets its isolated data; backend rebuilds within that session preserve it.

Sign in as `observer` with the same initial password to try basic server operation
and read-only schedules on Survival. The enable switches are disabled, while schedule
details and execution history remain accessible. The `viewer` account remains
unassigned for access-denial testing. Existing mock databases receive the observer
fixture on the next backend restart without resetting their saved data.

```sh
make dev                 # API and Vite with real Spark connections
make test                # Go race tests, type checks, and frontend behavior tests
make lint                # Go and frontend lint
make format-check        # Go and frontend formatting checks
make build               # Panel with embedded UI, Spark, and installer bundles
make check               # Full formatting, vet, lint, test, and build checks
make test-e2e-docker     # Browser tests and strict screenshot comparisons
```

The binaries are written to `bin/campfire` and `bin/spark`. Production hosts do not need Node.js or pnpm. Tagged builds package Linux amd64 and arm64 binaries, the project README, installation files, and SHA-256 checksums. Supplied systemd units are available for the [panel](https://github.com/dougmaitelli/Campfire/blob/master/deploy/systemd/campfire.service) and [Spark](https://github.com/dougmaitelli/Campfire/blob/master/deploy/systemd/spark.service).

See the [browser testing guide](https://github.com/dougmaitelli/Campfire/blob/master/e2e/README.md) for the contributor testing workflow.

## Mock server scenarios

The eight-server fleet combines useful cases rather than creating every possible
combination. Existing examples are retained; only three servers are added for the
configuration states that need separate running or delivery baselines.

| Server                    | Scenarios to exercise                                                                                                                                                                                                                    |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Survival                  | Healthy running server, game publications and media, Discord notifications/card, file management, scheduled restart, owner and observer access                                                                                           |
| Creative                  | Stopped server, EULA log alert and file action, named startup commands, full/save-data archives and retention, save paths changed without review or deployment sync                                                                      |
| Modded                    | Ready for installation, operator/editor access differences, Spark without hard disk quotas                                                                                                                                               |
| Unavailable               | Offline Spark, unavailable metrics, queued operations                                                                                                                                                                                    |
| Ember update ready · Mock | Running server awaiting value review: image/startup selection, changed locked value, retained custom MOTD, added empty and boolean values, private changes and removed values; two container ports using TCP and UDP; backups still work |
| Restart required · Mock   | Saved custom startup and memory differ from the running container; game integration disabled, unlimited disk; restarting applies the changes                                                                                             |
| Deployment sync · Mock    | Connected Spark rejects configuration delivery while status, console and backups remain available; recovery leads to a pending restart                                                                                                   |
| First sync · Mock         | Not installed, no configuration acknowledgement yet; transfer can be queued before the first successful sync                                                                                                                             |

In **Deployment sync · Mock**, send `mock-config resume` through its console to
recover configuration delivery for both servers on **Configuration recovery**.
The collector retries automatically; **Sync deployment** also retries immediately.
The running server then needs a restart, while **First sync · Mock** remains
uninstalled. `mock-config pause` blocks future deliveries on that simulated Spark.
This affects configuration delivery only, not the other mock transports.

The new save path on Creative is `whitelist.json`. A new save-data backup includes
it immediately; existing archives retain their original paths. Both backup types
also remain available on the pending-review example.

Use `casey` with the shared mock password for status-only access to the sync and
restart examples: configuration and power controls stay hidden. `sam` can operate
and synchronize the sync example without editing its settings. Existing `alex`,
`morgan`, `jordan`, `observer`, and unassigned `viewer` examples retain their roles.

Fixture setup runs once and does not overwrite subsequent edits. Delivery pause
settings, lifecycle transitions, and the simulated running-container fingerprint
survive backend rebuilds. A new mock-dev session resets them. These additions use
only `runtime_records.active_revision` and
`spark_client_records.configuration_blocked` in the isolated mock database;
production table definitions, API fields, permissions and wire contracts are
unchanged. The deterministic browser fixture fleet is separate and does not gain
these servers or screenshot baselines.

## Mock game integration

For mod and adapter authors, the [game integration developer guide](/Campfire/game-integrations/)
documents the publishing API, payload schemas, delivery guarantees, and client
implementation patterns. The simulator below helps develop panel features; it
does not replace testing a mod against a real Spark socket.

`make dev-mock` automatically simulates game publications for running, online
servers. State heartbeats arrive every five seconds. Every fifteen seconds the
publisher advances through a connection announcement, Alex joining, a welcome
announcement, and Alex leaving. Player count and capacity appear in monitoring;
optional game mode is omitted while Alex is online to exercise snapshot
replacement. The welcome event includes a synthetic PNG screenshot and a
one-second WebM clip. These are generic development scenarios, not a Valheim mod.

The existing **Creative** server starts stopped with a simulated Minecraft
EULA issue under **Needs attention**. It includes matched output, an **Open eula.txt**
action, and dismissal. The rule belongs to **Minecraft · Current**.
This example is added to existing mock databases on restart;
dismissal survives subsequent restarts. To trigger it again after starting a new
run, send `You need to agree to the EULA` in that server's mock console.

Use the normal server console to control a publisher:

| Command            | Behavior                                                          |
| ------------------ | ----------------------------------------------------------------- |
| `mock-game pause`  | Stop publishing; current state becomes stale after 60 seconds.    |
| `mock-game resume` | Resume heartbeats and the event cycle.                            |
| `mock-game next`   | Advance to the next event on the next heartbeat, unless paused.   |
| `mock-game replay` | Resend the last event with the same ID to exercise deduplication. |

Stopping a server stops its publisher. Starting or restarting it begins a new
session with new event IDs. Existing history remains in the database. Publications
use the real game ingestion, placement checks, state projection, and event store;
the simulator bypasses the game socket and Spark queue. Real transport behavior
is covered separately by Spark tests. The deterministic browser screenshot
fixtures do not start this background publisher.

## Mock Discord

`make dev-mock` uses a local fake Discord API. It never sends integration requests
to real Discord, even if a real token is entered. Startup prints the panel URL,
the fake bot token, and the viewer URL:

```text
Panel:        http://127.0.0.1:8081
Mock Discord: http://127.0.0.1:8080/mock/discord
Bot token:    campfire-test-discord-token
```

Fresh sessions already connect **Campfire Community** and enable Survival's
game/power notifications and server card in **server-updates**. Open the viewer
to see messages. Open **Integrations** from the panel account menu to edit,
disable, or remove the connection. These changes survive backend rebuilds.
To add it again, choose **Add integration → Discord**, enter the fake token, and
select **Campfire Community** once servers load automatically. In a game's console,
open **Actions → Edit server → Integrations** to select notifications and card fields. Use
`mock-game next`, power actions, or backups to exercise the normal event pipeline.

The viewer displays the actual outgoing embeds by channel. Card edits keep the
same message ID and gain an edited indicator. **Delete message** simulates removal
in Discord; an enabled card is recreated after its next existence check, normally
within about a minute. Notifications are not recreated when deleted.

**Channel behavior** can simulate missing permissions, five-second rate limits,
or an unavailable provider for either writable channel. Other channels continue
working. Return the channel to **Normal**, let any rate-limit cooldown expire,
and retry failed work in the panel. **read-only** always lacks Send Messages.

The simulated inbox and failure settings are memory-only and reset when the
backend rebuilds. Saved panel integration settings survive those rebuilds;
existing cards are reconciled against the empty fake provider. A new mock-dev
session resets all isolated state as usual.

The provider, embedded viewer, and mock startup adapter live under
`devfixture/integrations` and `devmock/integrations`, behind `!production` build
constraints. The regular panel does not mount these routes or support an
environment setting to redirect Discord. No database or public API changes are
needed for this development tool. A real bot in a private Discord server can
still be used with regular `make dev` for a final live smoke test.

## Tests

Frontend tests run through Vitest and import application TypeScript directly using
the Vite toolchain:

- `web/tests/unit/`: isolated logic such as API response handling, routing,
  operation messages, console streams, and timestamp formatting.
- `web/tests/functional/`: component behavior with Testing Library and jsdom,
  including user interactions, permissions, translations, and installer execution
  with temporary fixtures.
- `e2e/`: Playwright flows against the built application and real API. Native
  browser behavior such as modal dialogs and dismissal belongs here.

Backend unit and functional tests live beside the Go packages as `*_test.go`.
API and repository tests exercise authorization and persistence boundaries.

```sh
pnpm --dir web run test:unit
pnpm --dir web run test:functional
pnpm --dir web run test          # All frontend unit and functional tests
make test                      # Backend race tests and frontend checks
make test-e2e-docker            # Browser flows and screenshot comparisons
```

`make test-frontend` and CI include test type checking and the complete Vitest
suite. Formatting and lint checks cover the test files and configuration as well.
Add application coverage to these suites. Keep `scripts/` for development, build,
release, and operational tooling.

## Documentation site

The Astro and Starlight site is a separate package in `docs/`:

```sh
make dependencies-docs
make docs-dev
```

Open the `/Campfire/` URL printed by Astro. To validate the production output:

```sh
make docs-check
make docs-build
make docs-preview
```

`make docs-check` checks formatting, ESLint, and Astro diagnostics. Use
`pnpm --dir docs run format` to format documentation sources locally.

The build generates the site and search data for `/Campfire/`. Source screenshots
come from the root `screenshots/` directory.

Pull requests validate the site. Pushes to `master` build and deploy it through
GitHub Actions; the repository's Pages source must be set to **GitHub Actions**.
Version tags validate the docs without replacing the live site.
