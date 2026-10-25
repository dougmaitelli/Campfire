---
title: Development
---

Use Go 1.26 or newer, a C compiler for SQLite/CGO, Node.js 26, pnpm 11.28.5, and Make. Docker is needed for real game execution and the canonical browser test environment.

```sh
make dependencies
make dev-mock
```

Open `http://127.0.0.1:8081` and sign in with `admin` / `campfire-dev-password`. Mock mode runs the real authentication, API, database, and services against simulated Sparks. It includes sample servers, roles, notifications, files, console output, and backups without needing Docker or production credentials. Each new `make dev-mock` session resets its isolated data; backend rebuilds within that session preserve it.

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
