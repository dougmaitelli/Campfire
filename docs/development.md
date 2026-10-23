# Development

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

The binaries are written to `bin/campfire` and `bin/spark`. Production hosts do not need Node.js or pnpm. Tagged builds package Linux amd64 and arm64 binaries, the project README, installation files, and SHA-256 checksums. Supplied systemd units are available for the [panel](../deploy/systemd/campfire.service) and [Spark](../deploy/systemd/spark.service).

See the [browser testing guide](../e2e/README.md) for the contributor testing workflow.
