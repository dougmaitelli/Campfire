---
title: Local development
description: Run Campfire locally, exercise mock scenarios, and run the test suites.
---

Use Go 1.26 or newer, a C compiler for SQLite/CGO, Node.js 26, pnpm 11.28.5, and
Make. Docker is needed for real game execution and the canonical browser tests.

## Start the application

```sh
make dependencies
make dev-mock
```

Open `http://127.0.0.1:8081` and sign in with `admin` / `campfire-dev-password`.
Mock mode runs the real panel against simulated Sparks, with sample servers,
users, files, backups, and notifications. No Docker or production credentials
are needed. Each new session resets its isolated data; backend rebuilds within
a session preserve panel changes.

Use `make dev` instead to work with real Spark connections.

The `campfire-mock` entry point composes the panel with
`backend/internal/devmock`. Reusable development and test fixtures live in
`backend/internal/devfixture`; both packages are excluded by the `production`
build tag. The simulator can call panel and Spark implementations directly without
making them shared production packages. Neither regular executable imports the
simulator, even in an untagged development build. The browser harness uses the
same fixtures but keeps its deterministic runtime separate from interactive mock
scenarios.

## Mock scenarios

| Server                    | Useful for testing                                                                 |
| ------------------------- | ---------------------------------------------------------------------------------- |
| Survival                  | Running-server controls, files, schedules, game events/media, and Discord delivery |
| Creative                  | Stopped-server controls, EULA alerts, and full/save-data backups                   |
| Modded                    | First installation and differences between operator and editor access              |
| Unavailable               | Offline-host behavior and queued operations                                        |
| Ember update ready · Mock | Reviewing an updated game definition                                               |
| Restart required · Mock   | Saved configuration that needs a restart                                           |
| Deployment sync · Mock    | Recovering failed configuration delivery                                           |
| First sync · Mock         | A server awaiting its first configuration delivery                                 |

All sample accounts use the shared mock password. Use `observer` for basic
operation and read-only schedules on Survival, `viewer` for an account with no
assigned servers, and `casey` for status-only access to the sync/restart examples.
`sam` can operate and synchronize the sync example without editing its settings.

For configuration-delivery recovery, send `mock-config resume` in the
**Deployment sync · Mock** console, then wait for the retry or choose **Sync
deployment**. Use `mock-config pause` to block delivery again. This affects both
servers on the **Configuration recovery** Spark without blocking other operations.

### History pagination

Interactive dev-mock adds older history to exercise numbered pages without
changing the shared browser-test and screenshot fixtures:

| History             | Additional examples                                                                   | Useful checks                                                                                                                 |
| ------------------- | ------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| Jobs                | 125 completed/cancelled copy and transfer jobs                                        | Three pages; **Copy + Completed** also spans multiple pages. Existing pending, running, and failed examples remain available. |
| Notifications       | 125 read/unread account and Survival messages with varied severities                  | Page replacement, unread counts, and **Mark all read** from page two.                                                         |
| Schedule executions | 125 runs across existing and retired schedules/servers                                | Global filters and retained-history access. **Nightly restart** alone has over 50 runs for its details drawer.                |
| Survival Activity   | 125 historical game events, plus the jobs, executions, and server notifications above | Mixed-source pages; **Game + player.joined** also spans multiple pages.                                                       |
| Audit               | 125 entries with different actions, targets, and outcomes                             | **server.start + server:survival** spans multiple pages; test filter resets and browser Back/Forward.                         |

Administrators can see **Retired realm · Pagination** in execution history even
though it has no current server record. Other accounts remain limited to servers
they can currently access. Some Survival runs reference a retired schedule so
schedule deletion can be tested independently of server deletion.

Bulk jobs are terminal and never reserve servers or run background work. Historical
records do not queue external notifications or game publications. New examples
are seeded once per mock database: backend rebuilds preserve read state, edits,
and deleted examples rather than adding duplicates or recreating servers. A new
mock session starts with fresh fixtures.

### Game publications and alerts

Running, online mock servers publish game state and a repeating cycle of player
and announcement events, including a sample screenshot and video clip. Use the
server console to control the publisher:

| Command            | Behavior                                                        |
| ------------------ | --------------------------------------------------------------- |
| `mock-game pause`  | Stop publishing; state becomes stale after 60 seconds.          |
| `mock-game resume` | Resume publishing.                                              |
| `mock-game next`   | Advance to the next event on the next heartbeat, unless paused. |
| `mock-game replay` | Resend the last event with the same ID to test deduplication.   |

Stopping the server stops its publisher; starting or restarting begins a new
session. Creative includes a sample EULA issue under **Needs attention**. To
trigger it in a new run, send `You need to agree to the EULA` in its mock console.

The simulator is useful for panel development but does not exercise the real
Spark socket. Mod and adapter authors should use the
[game integration guide](/Campfire/game-integrations/) and test against a real Spark.

### Discord

Mock mode uses a local fake Discord API and never sends integration requests to
real Discord. Startup prints the viewer URL and test token:

```text
Mock Discord: http://127.0.0.1:8080/mock/discord
Bot token:    campfire-test-discord-token
```

Survival starts with notifications and a live card in **server-updates**. Use
`mock-game next`, power actions, or backups to generate messages. The viewer shows
embeds, images, and playable clips. **Delete message** can test recreation of a
missing live card; ordinary notifications are not recreated.

Use **Channel behavior** to simulate missing permissions, rate limits, or an
unavailable provider. Return the channel to **Normal** before retrying failed
work. The **read-only** channel always lacks Send Messages.

The simulated inbox and failure settings reset on backend rebuilds; saved panel
integration settings survive. A new mock session resets both. Use `make dev`
with a real bot in a private Discord server for a live integration check.

## Tests and checks

| Suite                     | Location                            | What to cover                                                                            |
| ------------------------- | ----------------------------------- | ---------------------------------------------------------------------------------------- |
| Go tests                  | `*_test.go` beside backend packages | Backend behavior, authorization, and persistence                                         |
| Frontend unit tests       | `web/tests/unit/`                   | Isolated TypeScript logic                                                                |
| Frontend functional tests | `web/tests/functional/`             | Component behavior and interactions with Testing Library and jsdom                       |
| Browser tests             | `e2e/`                              | Playwright flows against the built application, native browser behavior, and screenshots |

```sh
pnpm --dir web run test:unit
pnpm --dir web run test:functional
make test                 # Go race tests, frontend type checks, and behavior tests
make test-e2e-docker       # Browser flows and strict screenshot comparisons
make lint                 # Go and frontend lint
make format-check         # Go and frontend formatting checks
make check                # Full formatting, vet, lint, test, and build checks
```

Add coverage to the suite that matches the behavior being changed. See the
[browser testing guide](https://github.com/dougmaitelli/Campfire/blob/master/e2e/README.md)
for running targeted browser tests and maintaining screenshot baselines.

## Build

Run `make build` to produce `bin/campfire` and `bin/spark`, including the embedded
UI and installer bundles. Production hosts do not need Node.js or pnpm.

## Documentation site

The Astro and Starlight site is a separate package in `docs/`:

```sh
make dependencies-docs
make docs-dev
```

Open the `/Campfire/` URL printed by Astro. Changes reload automatically.

```sh
make docs-check           # Formatting, ESLint, and Astro diagnostics
make docs-build           # Production site and search index
make docs-preview         # Preview the production build
```

Use `pnpm --dir docs run format` to format documentation sources. Screenshots come
from the root `screenshots/` directory.

Pull requests validate the site. Pushes to `master` deploy it through GitHub
Actions; the repository's Pages source must be **GitHub Actions**. Version tags
validate the docs without replacing the live site.
