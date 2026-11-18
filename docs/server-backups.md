---
title: Game backups
---

Use a server's **Backups** tab to create, download, restore, or delete snapshots.
The main **Backups** page is an archive library. Backups have stable identities
and an optional server association; an archive does not need a server to exist.
Associated archives follow that server's backup permissions. Independent uploads
belong to their uploader and are also available to administrators. Archives
detached by server deletion have administrator custody, not former server-member
access. API-token scopes and View-as restrictions apply to all of these paths.
Storage indicators distinguish local storage (panel or Spark) from off-host S3.

Creating, restoring, downloading, and deleting backups require separate grants;
backup policy requires `server.configure`. See
[Accounts and access](/Campfire/authentication/#permission-hierarchy) for the
permissions and Backup maintainer preset.

## Choose a backup type

| Type               | Contents                                                   |
| ------------------ | ---------------------------------------------------------- |
| **Full server**    | The whole game-data directory                              |
| **Save data only** | Files and folders declared by the Ember's `savedata_paths` |

Choose the type for each manual backup or backup schedule; there is no server-wide
default. Save-data backups require the latest Ember to declare save paths. Pending
server-value reviews do not block either type. Path changes apply to new backup
requests immediately, without deployment sync or restart.

## Create a backup

Backups can run while the server is running. Live backups copy files as they are
read; they do not flush in-memory game saves or provide an atomic snapshot across
files. Save the game first when possible, or stop it for a consistent backup.
Missing required save paths fail backup creation; missing optional paths are
recorded in the archive.

### Scheduled backups

Backup schedules offer **Stop server during backup**. When selected, Spark stops
a running server, creates the backup, and attempts to start it again even if
backup creation fails. A server that was already stopped remains stopped. The
sequence runs as one operation on Spark and continues if the panel restarts.
Follow its execution history and server status to confirm both the backup and
restart completed. See [Schedules](/Campfire/schedules/) to configure recurring work.

## Restore a backup

Every restore asks for confirmation and explains which files will be replaced.
If the server is running, the dialog offers to stop it, restore the backup, and
restart it. Clear **Restart server after restoring** to leave it stopped instead.
The panel waits for each operation to succeed before starting the next. Keep the
page open until the sequence finishes. A failed restore never starts the server.

A full restore replaces all game files. A save-data restore replaces only the
captured paths, including removing files added under those paths since the backup.
Other installed files remain in place. Reinstall a compatible game version before
restoring older save-data archives onto an empty server. Portable save-data backups
install the game first when the destination is not installed. A server that was
already stopped remains stopped after either restore type. Check its console
after starting it or completing a restore with restart selected.

Each archive records its type and selected paths. Restoring an older backup uses
that snapshot's paths even if the Ember has changed.

Keep enough host space for the current game directory and a staged replacement
at the same time. Save-data restores also stage the installed files they preserve.
Restores assign files to the server's runtime user and use private, writable
permissions: `0600` for files and `0700` for directories and executable files.
This prevents read-only permissions from blocking future saves.

## Portable backups and recovery

When saved server values match the latest Ember, new backups also include the
server settings and the complete Ember definition.
Hidden variable values, including passwords and tokens, are included for exact
recreation. Treat downloaded archives as sensitive credentials and store them
privately. Import previews do not show hidden values.

To recover into an existing server, choose **Upload backup** in its **Backups**
tab. After upload and validation, the archive appears in that server's catalog.
Stop the server, then choose **Restore**. Its current settings, ownership, and
access grants remain in place. A full restore replaces its game filesystem;
a save-data restore replaces only the paths captured in the archive.

To recreate a server, choose **Create from backup** in an existing portable archive's
**…** actions menu in **Backups**, or choose **Create from backup** in the **Servers** action menu and
upload an archive. Both use the same inspection and server-creation workflow.
The upload is finalized into the library before the creation wizard uses it.
Cancelling the wizard does not discard the archive. Creating a server copies the
backup into the destination's catalog; it does not consume the original archive
or change its association. Review the recovered settings, select a destination Spark, and allocate
new host ports. Ownership and access grants are configured on the destination;
source placement and grants are not copied. A full backup restores the filesystem
without running the installer. A save-data backup installs game files before
restoring the captured paths. The new server remains stopped after restoration.

An administrator can import the included Ember if no matching definition exists.
Its installer is executable code: only import backups from a trusted source.
Other server creators can use backups whose Ember already exists in the panel.
Recreating non-default hidden variable values requires an administrator, matching
the existing restriction on changing hidden variables.
Uploading requires backup creation permission; restoration requires the separate
backup restore permission. Creating from backup also requires server creation
access to the destination Spark.

Portable imports require embedded server settings and an Ember definition and
can come from another Campfire installation. Data-only archives can be restored
from their existing catalog but cannot be uploaded through this workflow. Review
pending server-value changes before creating a portable snapshot. Backups made while that
review is pending still protect data but omit portable server settings.
Use **Upload backup** on the main **Backups** page to store an archive without
creating a server. Uploading an independent archive requires server-creation
permission. Reading an existing archive for creation requires backup-download
permission as well as permission to create the destination server.

Uploads targeting an existing server finalize only in that server's Spark backup
store. The panel temporarily stages and validates the upload, then removes its
staging file after publication or cancellation. It does not keep an additional
independent copy. The Spark archive still appears in the shared Backups catalog
with its own ID and server association. Uploads from the main Backups page and
Create from backup instead share the panel's durable `backups/` store.

Browser uploads use the configured file-upload limit. Incomplete upload sessions
and preparation receipts expire after one hour; finalized library archives do not.
Stored archives are inspected and copied in bounded chunks rather than requiring
another browser upload or a second full temporary copy on the panel. Their
internal delivery is bounded by the archive limit, not the browser-upload limit.
Keep enough free space on the panel for stored uploads and on Spark for delivered
archives plus staged restoration.

The shared upload-session implementation owns temporary files, chunk validation,
completion checks, and cleanup for game files and backup uploads. Destination
validation and publication remain operation-specific. Panel staging lives in
`uploads/` under its state directory; finalized uploads live in `backups/`.
Finalization keeps a durable archive link before removing temporary session state.
Expired abandoned staging files are cleaned on subsequent upload admission;
finalized archives are never included in that sweep.

## Name and pin snapshots

Use a backup's actions menu to rename or pin it from the server's **Backups** tab
or the main **Backups** page. These actions require `server.backups.create`.
Names are stored in the catalog; archive filenames and download/restore references
remain stable. Metadata can be edited while its Spark is disconnected.

Pinned backups keep their existing local and S3 copies regardless of age or count
limits and do not consume the retention slots reserved for ordinary backups.
When S3 synchronization is enabled, pinned local backups also sync regardless of
their age. Unpin a snapshot to make normal retention rules apply again; an old
snapshot may then be removed during the next cleanup. Unpinning is also required
before manually deleting a backup. Deleting a server preserves its backups and
their pins; it only clears their server association. Unlinked archives do not
inherit server retention policies when unpinned.

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
displace retained full snapshots. Zero disables each individual limit.
The panel and Sparks must be able to reach and trust the storage endpoint for
upload and restore workflows.

## Server deletion and independent archives

Deleting a server removes its containers and game data, but preserves local and
S3 archives. The catalog clears `server_id` transactionally with removal of the
server record. Existing independent archives are untouched. There is no deleted-
server/origin table: private storage locators locate the bytes, while the public
archive has only its own metadata and optional association to a live server.

Deletion inventories local archives first. If Spark is offline or its archive
store cannot be inspected, deletion fails without forgetting the server or its
backups; reconnect and retry. Backup preservation is the default deletion behavior
under the existing lifecycle capability, with no separate capability or opt-in.
Updated Sparks never remove the backup directory during deletion or retries.
Upgrade older Sparks before deleting servers: older binaries may still remove
archives, and the capability handshake does not distinguish that behavior.

An uninstalled server with pending Ember value changes must first complete its
Ember review, so Spark can receive the configuration needed for deletion. This
preflight failure does not mark deletion as pending or prevent configuration
review. Installed servers can still be deleted using their saved configuration
without adopting pending Ember changes.

Before dispatching deletion, the panel persists `servers.deletion_pending`.
Retention and conflicting server operations remain blocked if the deletion response is
lost, database cleanup fails, or the panel restarts. Retry server deletion to
complete it; the intent disappears with the server record. It is transient
lifecycle state, not a permanent archive-preservation marker or deleted-server
entity. Archive mutation claims also check the custody state that was authorized,
so an in-flight server-based grant cannot mutate an archive after detachment.

Copy deletion checkpoints are saved before attempting another copy. If remote
cleanup cannot be confirmed in the catalog, server deletion stops before setting
its lifecycle intent, allowing the existing cleanup worker to retry. Once that
checkpoint is settled, deleting the server preserves any remaining local copy
without advertising a removed S3 copy.

Independent archives have no automatic server-policy expiry or orphan cleanup.
Their sizes remain in the catalog and their bytes continue consuming panel/Spark
filesystem or S3 capacity, outside the deleted server's game-disk allocation.
Monitor that storage and explicitly delete unneeded archives. Pins must first be
removed. Partial explicit deletion checkpoints completed copy removal; independent
archives require an explicit retry for remaining unavailable copies. A Spark's
identity cannot be removed while it stores independent local archives: export or
copy them elsewhere as needed, then explicitly delete them before removing that
Spark. The Backups storage column identifies the current storage Spark when
known. Search by its name or ID, or use `spark:<id>` to match that storage Spark
exactly, including rows marked **No server**. The blocked-removal error supplies
this exact filter. Archive IDs are
also searchable. These are current storage labels, not historical source-server
metadata. Private filesystem paths and storage keys remain hidden, and normal
archive permissions still determine which search results are visible.

The database upgrade assigns stable backup IDs, preserves existing object keys and
metadata, and makes `server_id` nullable. Legacy orphan records remain visible to
administrators. If their local storage location was already lost before this
upgrade, Campfire does not guess it or purge their files; recover those bytes from
the original host. Configured S3 copies remain downloadable.

Game snapshots do not replace backups of the panel database, its finalized upload
library, and node identities; see [installation backups and upgrades](/Campfire/operations/).
If catalog registration cannot be confirmed after an upload is durably stored,
Campfire keeps the file rather than risking deletion of a committed archive.
Recover unregistered files from the panel state's `backups/` directory by importing
them again; do not sweep that directory as temporary upload storage.
