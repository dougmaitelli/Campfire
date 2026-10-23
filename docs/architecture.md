# Architecture

Campfire has two production executables: the panel (`campfire`) and the game-host
agent (`spark`). Every game host runs a separate Spark, even when the panel and
games share one machine. The panel has no Docker dependency.

The browser connects to the panel's web UI and API. Sparks initiate outbound TLS
connections to the panel's dedicated agent listener. Enrollment establishes each
Spark's identity; subsequent management connections use client certificates.
Players connect directly to the game ports on the hosting Spark.

## Responsibilities

| Component | Owns                                                                                                                |
| --------- | ------------------------------------------------------------------------------------------------------------------- |
| Panel     | Accounts, authorization, saved configuration, schedules, audit history, notifications, and coordination with Sparks |
| Spark     | Docker containers, game installation, console output, game files, local backups, and resource measurements          |
| Ember     | The imported definition of a game's images, installer, startup, variables, ports, and configuration rules           |

The panel stores application records in SQLite or PostgreSQL, with private
identity material in its persistent data directory. Each Spark keeps its own
identity, delivered configuration, game data, and local backups on the game host.

## Repository layout

| Path                         | Contents                                                                       |
| ---------------------------- | ------------------------------------------------------------------------------ |
| `backend/cmd/`               | Production panel and Spark entry points, plus the development mock entry point |
| `backend/internal/campfire/` | Panel API, services, repositories, agent coordination, and embedded UI assets  |
| `backend/internal/spark/`    | Agent connections and game runtime                                             |
| `backend/internal/shared/`   | Shared models, protocol, and release metadata                                  |
| `web/`                       | React and TypeScript frontend                                                  |
| `config/`, `deploy/`         | Configuration examples and systemd units                                       |
| `scripts/`                   | Development, build, release, and verification tooling                          |
| `e2e/`, `backend/e2e/`       | Browser tests and their isolated Go harness                                    |
| `screenshots/`               | Images displayed in the README and checked by browser tests                    |

HTTP handlers delegate application behavior to services; repositories handle
database access. The browser and panel send requests to the Spark assigned to a
server, and the Spark executes them against its local Docker host and files.
