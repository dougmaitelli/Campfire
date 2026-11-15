---
title: Architecture
description: Understand Campfire's components, trust boundaries, storage, and execution model.
---

## Repository layout

| Path                         | Contents                                                                      |
| ---------------------------- | ----------------------------------------------------------------------------- |
| `backend/cmd/`               | Panel, Spark, and development mock entry points                               |
| `backend/internal/campfire/` | Panel API, services, repositories, agent coordination, and embedded UI assets |
| `backend/internal/spark/`    | Agent connections and game runtime                                            |
| `backend/internal/shared/`   | Shared models, protocol, and release metadata                                 |
| `web/`                       | React and TypeScript frontend                                                 |
| `config/`, `deploy/`         | Configuration examples and systemd units                                      |
| `scripts/`                   | Development, build, release, and verification tooling                         |
| `e2e/`, `backend/e2e/`       | Browser tests and their isolated Go harness                                   |
| `screenshots/`               | README images and browser-test baselines                                      |
| `docs/`                      | Documentation source and site configuration                                   |

HTTP handlers delegate application behavior to services; repositories handle
database access. Agent coordination sends authorized execution requests to the
Spark assigned to a server, where runtime code operates on Docker and local files.
For setup and testing commands, see [Local development](/Campfire/development/).

## System overview

Campfire has two production executables: the panel (`campfire`) and the game-host
agent (`spark`). Every game host runs a Spark, even when the panel and games share
one machine. The panel has no Docker dependency.

```text
Browser ── HTTPS ──> Campfire panel ──> SQLite or PostgreSQL
                         ▲
                         │ Spark-initiated TLS connection
                         │ authenticated with client certificates
                       Spark ──> Docker containers and game files
                         ▲                  ▲
                         │ private socket   │ game ports
                    Game publisher       Players
```

Browser requests go through the panel; the browser does not communicate directly
with Spark. Sparks initiate outbound connections to the panel's dedicated agent
listener. Players connect directly to game ports on the hosting Spark, not through
the panel.

## Responsibilities and trust boundaries

| Component | Responsibility                                                                                                |
| --------- | ------------------------------------------------------------------------------------------------------------- |
| Panel     | Accounts, authorization, saved configuration, schedules, history, notifications, and coordination with Sparks |
| Spark     | Local Docker execution, installation, console output, game files, backups, and resource measurements          |
| Ember     | Reusable definition of a game's images, installer, startup, variables, ports, and configuration rules         |

The panel authorizes user and API-token requests before dispatching work. Server
ownership and membership determine resource access; token scopes can narrow that
access. See [Accounts and access](/Campfire/authentication/).

Enrollment establishes each Spark's identity. Subsequent management connections
use client certificates. The panel checks that an agent is authorized for the
server it reports or operates on; Spark validates execution inputs and confines
file operations to the server's data directory.

Game publishers have a separate boundary: an opted-in game container receives
only its own private Unix socket. It does not receive panel tokens or Spark
credentials. Spark supplies the server identity, and Campfire validates placement
and whether publishing is enabled. Host administrators remain trusted: they can
access local game files and sockets.

Ember installers and startup commands execute code on game hosts. Definitions
and portable backups containing definitions must therefore come from trusted
sources. See [Security](/Campfire/security/).

## State and storage

| Location             | Stored data                                                                                                                             |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| Panel database       | Accounts, permissions, server configuration, Ember definitions, schedules, retained history, jobs, notifications, and game events/media |
| Panel data directory | Private signing authority, identities, SFTP host key, and encrypted integration credentials with their encryption key                   |
| Spark data directory | Agent identity, delivered execution configuration, runtime metadata, queued game events, game files, and local backups                  |
| Optional S3 storage  | Off-host backup archives                                                                                                                |
| Panel memory         | Latest game-state snapshots; these are not historical records and are cleared on restart                                                |

SQLite and PostgreSQL store the same application records. Neither replaces the
panel's private filesystem state: a database-only backup cannot recover the
signing authority or decrypt integration credentials. Likewise, panel state does
not replace Spark identities or game-data backups. See
[Upgrades and recovery](/Campfire/operations/).

Game events are durable records with retention limits. Current game state is a
replaceable, expiring snapshot, independent of runtime status. A running game can
have unavailable game data without being stopped. Backup archives are separate
from both forms of history.

## Configuration and execution

The panel owns the full server configuration and Ember definition. Before delivery,
it projects them into the execution data Spark needs: the selected image,
variables, resource limits, port bindings, resolved startup selection, installer,
stop behavior, readiness rules, and file-patching instructions.

Editing policies, image alternatives, display metadata, source attribution, and
import warnings remain on the panel. Startup selection is resolved there; Spark
expands the selected command's variable placeholders from the container environment,
keeping variable contents literal.

Validation happens at both boundaries. The panel enforces Ember policies and
variable constraints. Spark validates execution inputs such as users, paths,
images, resource limits, ports, and environment names before saving them.

### Synchronization and restart boundaries

Saving and delivering configuration does not implicitly restart a running game.
Spark distinguishes saved configuration from the configuration used by its running
container and reports when a restart is required.

The panel resolves the latest Ember against saved server values rather than
pinning each server to a historical definition. Changes requiring different saved
values need review; compatible definition changes can synchronize independently.
Backup save paths are evaluated separately from container execution settings.
See [Servers](/Campfire/servers/) for the editing workflow.

Spark and Campfire negotiate capabilities for individual operations. A connected
agent may support only part of the current feature set; connection health alone
does not imply that every action is available. See
[Game hosts](/Campfire/spark-agent/#connection-and-feature-troubleshooting).

## Long-running work and recovery

The panel coordinates work; Spark executes local container and filesystem
operations. Schedules are dispatched by the panel using current authorization,
while the assigned Spark performs the requested action. The panel must be running
to dispatch scheduled work.

Recovery depends on the operation, not on a blanket promise that every browser
request resumes. Copy and transfer jobs keep durable progress and recovery state
and can wait for disconnected Sparks. A retry resumes the existing job rather
than creating a second operation. Cancellation may require a Spark to reconnect
before cleanup completes. See [Copies and transfers](/Campfire/server-transfers/).

### Backups and restoration

Spark creates local archives and stages restored data before replacing game files.
Full backups capture the game directory; save-data backups capture the paths
specified by the Ember. Archive metadata records the backup type and captured
paths, so restoration does not infer behavior from filenames or the current Ember.

When requested, Spark performs stop, backup, and conditional restart as one
operation. It attempts to restart a previously running game even if backup
creation fails. Restoration requires a stopped game; browser-managed stop/restore/
restart sequences have different lifecycle guarantees from a Spark-owned backup
operation. See [scheduled backups](/Campfire/server-backups/#scheduled-backups)
and [restoration](/Campfire/server-backups/#restore-a-backup).

Portable backups also contain server settings and an Ember definition, including
hidden variable values, but not source placement or access grants. The panel
validates imports and transfers archive data to Spark for restoration. A portable
definition is omitted when saved values need review, without preventing a data
backup. See [Portable backups](/Campfire/server-backups/#portable-backups-and-recovery).

## Events and integrations

Spark monitors container output for readiness and actionable errors independently
of an open browser. Detection checkpoints survive agent restarts, and retained
Docker output can be replayed after interruption. Output already removed by log
rotation cannot be recovered. Rules belong to [Embers](/Campfire/ember-schema/#actionable-console-errors);
users respond through [console alerts](/Campfire/servers/#respond-to-console-alerts).

Game publishers use the local socket for registration, state, events, and media.
These have deliberately different delivery guarantees:

- **Registration** succeeds after Campfire stores the definitions.
- **State** succeeds after the panel accepts a snapshot; it is not queued offline.
- **Events** succeed locally after Spark persists a bounded queue entry, before
  panel delivery is necessarily complete.
- **Media** succeeds after Campfire commits the attachment; the publisher retains
  responsibility for retrying interrupted uploads.

See [Game integrations](/Campfire/game-integrations/) for the publishing contract,
limits, and error handling.

### Notification delivery

Source changes and their notification records commit together. External provider
requests run afterward through delivery workers, so a Discord or Apprise outage
does not undo the source event. Activity presents retained sources and their linked
notifications; personal inbox read state is separate.

Workers retry transient failures within bounds and preserve message identities
across restarts. A lost provider response is an ambiguous outcome: the message
may already exist. Recovery checks for a receipt rather than blindly creating a
duplicate; unresolved sends can remain paused. This is not an unlimited or
exactly-once delivery guarantee.

Discord tokens are encrypted in the panel's private state directory; the database
holds references, not the credential files or key. API responses and audit records
do not expose tokens. See [Discord integration](/Campfire/discord/) for provider
behavior and recovery, and [Notifications](/Campfire/notifications/) for monitoring
and Activity workflows.
