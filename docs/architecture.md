---
title: Architecture
---

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

## Spark execution configuration

The panel keeps the full `ServerConfiguration` and imported `Ember`. Before
delivery, `model.NewDeployment` projects them into `RuntimeConfiguration` and
`RuntimeDefinition`. The same projection is used for ordinary synchronization and
both ends of a server transfer.

| Execution data                                                           | Purpose                                                                      |
| ------------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| Server, Spark, and Ember IDs                                             | Placement checks and transfer identity                                       |
| Selected image, variable values, CPU, memory, disk, and port bindings    | Container execution and resource enforcement                                 |
| One resolved startup command, shell, working directory, and numeric user | Starting the selected game process                                           |
| Installer, stop command, readiness matchers, and file patch instructions | Installation, shutdown, readiness detection, and local configuration changes |
| Game integration flag                                                    | Access to the local game publishing socket                                   |

Startup selection and custom overrides are resolved by the panel. Spark receives
one command template and expands its variable placeholders through the container
environment; variable contents remain literal values. Image alternatives,
variable defaults and editing policies, display names, descriptions, artwork,
source attribution, and import warnings stay on the panel. If no port is
published, the panel resolves any required-port fallback into `SERVER_PORT`.

Spark validates execution inputs before saving them, including numeric users,
container paths, image references, resource limits, port bindings, environment
names, and configuration patch paths. The panel validates Ember policies and
variable constraints before projection.

The execution-only payload uses `server.configure.v2`. Custom startup commands
are part of this contract and have no separate capability. The wire protocol
version remains unchanged. Older Sparks can still connect and receive remote
Spark updates, but must be upgraded before receiving this configuration format.
Upgraded Sparks can read their existing full-definition snapshots from disk,
including named startup selections and custom overrides. New deliveries replace
those snapshots with the execution-only format.

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
