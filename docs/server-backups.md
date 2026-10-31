---
title: Game backups
---

Use a server's **Backups** tab to create, download, restore, or delete snapshots.
The main **Backups** page lists archives across the servers you can access.
Storage indicators distinguish local archives on Spark from off-host S3 copies.

`server.backups.read` allows listing backups and requires `server.operate`.
Creation, restoration, downloading, and deletion each have separate grants under
backup viewing: `server.backups.create`, `server.backups.restore`,
`server.backups.download`, and `server.backups.delete`. Restoration inherits server
operation without requiring file reading or writing. The Backup maintainer preset
selects backup creation, restoration, and downloading, with their prerequisites;
backup deletion must be granted separately. Backup storage and retention
settings remain part of `server.configure`. See [accounts and access](/Campfire/authentication/) for
the complete permission hierarchy.

## Create and restore

Stop the installed server before creating or restoring a backup. Follow the
operation status until completion; closing the console does not cancel the work.
For recurring backups, configure a schedule and arrange for the game to be
stopped before the backup action runs.

Choose **Full server** to capture the whole game-data directory, or **Save data
only** to capture the files and folders declared by the Ember's `savedata_paths`.
Save-data backups are available only when the applied Ember declares those paths.
Choose the type for each manual backup or backup schedule; there is no server-wide
default. Existing schedules continue to create full backups.

A full restore replaces all game files. A save-data restore replaces only the
captured paths, including removing files added under those paths since the backup.
Other installed files remain in place. Reinstall a compatible game version before
restoring save data onto an empty server. Both restore types leave the server
stopped; start it after restoration finishes and check its console.

Each archive records its type and selected paths. Restoring an older backup uses
that snapshot's paths even if the Ember has changed. Missing required paths fail
backup creation; missing optional paths are recorded. Archives created before
backup types were introduced restore as full snapshots.

New archives are named `full-<id>.tar.gz` or `savedata-<id>.tar.gz`. Their first
entry, `.campfire-backup.json`, records the mode, selected paths, missing optional
paths, and Ember revision. Restore uses this metadata rather than guessing from
the filename; older archive names remain supported.

Keep enough host space for the current game directory and a staged replacement
at the same time. Save-data restores also stage the installed files they preserve.

Restores assign ownership to the server's runtime user and normalize permissions
to `0600` for files and `0700` for directories and executable files. This prevents
read-only permissions from a migrated world from blocking future saves.

## Off-host storage and retention

Configure the S3 bucket, region, optional compatible endpoint, and credentials
on the panel using the [environment example](/Campfire/configuration/).
For Docker, add these settings to the Compose service; merely adding variables
to `.env` does not forward settings absent from `compose.yaml`.

Enable backup synchronization separately on each server and configure its local
and remote retention policies. Inspect synchronization errors before assuming an
archive has an off-host copy. Failed synchronization retains the local archive.
Configure local and S3 keep-last counts and maximum ages separately for **Full
server** and **Save data only** backups. Frequent save-data snapshots do not
displace retained full snapshots. Existing retention settings remain the full
backup limits; save-data limits start disabled. Zero disables each individual
limit.
The panel and Sparks must be able to reach and trust the storage endpoint for
upload and restore workflows.

Deleting a server also removes its local game data and backups. Keep a separate
recovery copy when retiring a server. Game snapshots do not replace backups of
the panel database and node identities; see [installation backups and upgrades](/Campfire/operations/).
