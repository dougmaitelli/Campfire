---
title: Upgrades and recovery
---

## What to preserve

Game snapshots and installation recovery protect different data. Preserve:

- The panel database and its persistent state directory, including the Spark signing authority and encrypted integration credentials
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

Database schema checks run at startup. A rollback may require restoring the
matching pre-upgrade database and state as well as the previous image or binary.

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
