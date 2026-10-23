# Browser tests and screenshots

The Playwright UI specs exercise the real built frontend, API, authentication,
and SQLite repositories. A standalone Go harness simulates only Spark operations.
The shared `backend/internal/campfire/devfixture` seed also powers dev-mock:
accounts, roles, nodes, servers, the Ember and artwork, notifications, console logs,
game files, and real archive contents come from the same definitions. Tests use
a fixed clock and isolated state; dev-mock keeps its live, persistent simulation.
It uses temporary databases, binds to loopback, and resets before every test.
The harness is excluded from production builds. Do not point these tests at a
development or production instance.

Browser specs, shared screen helpers, fixtures, Playwright configs,
package dependencies, runner scripts, and reports are self-contained in `e2e/`.
The Go harness stays in `backend/e2e/` so it can use the backend module and
internal packages. The root `screenshots/` directory contains the single set of
images used for both documentation and visual regression baselines.

From the repository root:

```sh
make test-e2e-docker
make screenshots-docker
```

Docker is the canonical rendering environment used by CI. The image takes its
Playwright version from the pinned dependency in `e2e/package.json`. It also
provides Go, Node, pnpm, Chromium, and browser dependencies.

For local runs with Go, Node, and pnpm installed:

```sh
pnpm --dir web install --frozen-lockfile
pnpm --dir e2e install --frozen-lockfile
pnpm --dir e2e exec playwright install chromium
make test-e2e
make screenshots
```

`screens.ts` defines shared page and drawer interactions. `ui.spec.ts` uses those
interactions for browser assertions and screenshot comparisons, including login,
server console, file browser, and per-server backup tabs. Add new screenshots to
these specs so documentation and regression tests always use the same UI states.

Visual baselines live in the root `screenshots/` directory. Tests fail on any
pixel difference and do not create missing baselines automatically. After an
intentional UI change:

```sh
make screenshots-docker
```

Review and commit the screenshot changes, then run `make test-e2e-docker` again.
`make screenshots-docker` runs the same specs with baseline updates enabled;
`make test-e2e-docker` only compares against those images. Use Docker for shared
baselines; local font and browser rendering can differ by OS.

Fixtures fix browser time, locale, timezone, and viewport; use stable IDs and seed
data; block unexpected external requests; and fail on browser exceptions and server
errors. Real session cookies survive a post-login fixture reset so screenshots do
not contain varying login audit timestamps. Tests run serially because they share
one disposable backend. No developer configuration or infrastructure credentials
are read by the harness.

CI publishes the HTML report, failure screenshots, traces, and videos as the
`browser-test-results` artifact. Locally, use `pnpm --dir e2e exec playwright
show-report` or `pnpm --dir e2e run test:e2e:ui` to investigate a failure.
