---
title: Upgrades and recovery
---

Game snapshots and installation recovery protect different data. Preserve:

- The panel database and its persistent state directory, including the Spark signing authority
- Each Spark's identity, runtime metadata, game data, and backup archives
- Any separately configured S3 backups and their retention settings

Before upgrading, stop the panel and take a consistent database and state backup. For SQLite, copying only a live database file can omit WAL contents. For PostgreSQL, back up the database as well as the panel state directory.

For Compose upgrades, select the desired image tag in your deployment configuration, then run `docker compose pull` and `docker compose up -d`. Check login, health, and Spark connections afterward. Upgrade Spark when required by protocol or feature compatibility; wait for active operations to finish before restarting an agent.

For binary upgrades, stop the relevant service, replace its binary with the verified new release, and restart it. Preserve the previous binary until verification succeeds. Database migrations run at startup; a rollback may require restoring the matching pre-upgrade database and state as well as the previous image or binary. Do not use `docker compose down -v` during upgrades: it deletes persistent volumes.

### Administrator password recovery

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
