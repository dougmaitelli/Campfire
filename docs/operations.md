---
title: Upgrades and recovery
---

## What to preserve

Game snapshots and installation recovery protect different data. Preserve:

- The panel database and its persistent state directory, including the Spark signing authority, encrypted integration credentials, and finalized backup-library uploads in `backups/`
- Each Spark's identity, runtime metadata, game data, and backup archives
- Any separately configured S3 backups and their retention settings

Before upgrading, stop the panel and take a consistent database and state backup. For SQLite, copying only a live database file can omit WAL contents. For PostgreSQL, back up the database as well as the panel state directory.

Discord delivery identities are stored in the database, while the token files
and encryption key live in `integration-secrets` under the panel state directory.
Restore them together; see [Discord backup and recovery](../discord/#back-up-and-restore).

## Docker Compose upgrades

Select the desired image tag in your deployment configuration, then run:

```sh
docker compose pull
docker compose up -d
```

Check login, health, and Spark connections afterward. Do not use
`docker compose down -v` during upgrades: it deletes persistent volumes.

## Binary upgrades

Stop the relevant service, replace its binary with the verified release, and
restart it. Preserve the previous binary until verification succeeds.

Upgrade Spark when required by protocol or feature compatibility. Wait for active
operations to finish before restarting an agent.

## Recovery and rollback

Database schema checks run at startup. Schema 6 accepts release markers 1–5,
upgrading transactionally on SQLite and PostgreSQL. Schema 5 added retained crash
records; schema 6 adds public status pages and their explicit server memberships.
The archive migration assigns IDs, preserves storage keys and pins, and makes
`server_id` nullable. Schema 3 archive IDs remain unchanged. Schema 4 also adds
`servers.deletion_pending` (not null, default false) to fence retention and other
mutations while a server deletion awaits completion or retry. It does not move
or delete archive bytes. Unsupported or incomplete
schemas fail closed rather than being silently repaired.

A rollback to an older binary requires the matching pre-upgrade database and
state backup; do not point it at the upgraded catalog. Upgrade Sparks before
using server deletion: backup preservation is the default behavior in updated
Sparks under the existing lifecycle capability, not a separately negotiated
feature. Older binaries may still delete backup directories; the capability
handshake does not distinguish that older behavior.

## Audit coverage

The Audit page records administrative actions, permission changes, console-command
submission (not command text), and browser backup/file downloads. SFTP records
file mutations, denied file actions, and one transfer record per opened download.
Downloads remain pending until they finish; partial, disconnected, or uncertain
transfers are marked interrupted. A successful transfer means the server finished
sending the data, not that the client saved it to disk.

Successful upload chunks, archive-progress polls, file metadata probes,
notification read acknowledgements, and local configuration previews are omitted.
Failures and permission denials on those routes are still recorded. Standalone
API file reads (including raw chunk reads outside a tracked download) remain
audited so selecting a different read endpoint cannot bypass the audit trail. Remote Ember
URL previews remain audited because they initiate an outbound request. Repeated
SFTP authentication failures are aggregated into rate-limited security logs, without
supplied credentials or usernames.

Audited mutations and download starts fail closed if their audit intent cannot be
stored. Routine successful polling and inbox acknowledgements do not depend on
audit storage. Accepted requests and later operation/job results remain separate
records where the operation has a tracked lifecycle.

## History retention when upgrading

Audit and completed operation history are retained for **90 days** by default.
Records older than the configured retention period are eligible for automatic
deletion. To keep them indefinitely, set both
`CAMPFIRE_AUDIT_RETENTION_DAYS=0` and `CAMPFIRE_OPERATION_RETENTION_DAYS=0` in the
panel environment. Before upgrading, check that your retention settings match
the history you want to preserve. See the
[settings reference](/Campfire/configuration/#activity-and-game-event-retention).

## Administrator password recovery

Save a new password of at least 15 characters in a private, mode-`0600` regular file. On a binary installation, run:

```sh
sudo campfire reset-password \
  --database /var/lib/campfire/panel.db \
  --password-file /private/new-password
```

For Docker, place the file in the container's writable data directory, readable only by its UID `10001`, then run:

```sh
docker compose exec campfire campfire reset-password \
  --password-file /var/lib/campfire/new-password
```

Use your configured database path or PostgreSQL URL for binary installations. Add `--username NAME` to select an administrator; otherwise the first administrator is recovered. Recovery revokes that account's sessions. Remove temporary password files afterward. If local login is disabled, re-enable it in your deployment configuration before signing in with the recovered password.
