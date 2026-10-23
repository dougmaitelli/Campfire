# Game backups

Use a server's **Backups** tab to create, download, restore, or delete snapshots.
The main **Backups** page lists archives across the servers you can access.
Storage indicators distinguish local archives on Spark from off-host S3 copies.

## Create and restore

Stop the installed server before creating or restoring a backup. Follow the
operation status until completion; closing the console does not cancel the work.
For recurring backups, configure a schedule and arrange for the game to be
stopped before the backup action runs.

Restoration replaces the game files and leaves the server stopped. Choose the
snapshot carefully, and keep enough host space for the current game directory
and the extracted snapshot at the same time. Start the game after restoration
finishes and check its console.

## Off-host storage and retention

Configure the S3 bucket, region, optional compatible endpoint, and credentials
on the panel using the [environment example](../config/campfire.env.example).
For Docker, add these settings to the Compose service; merely adding variables
to `.env` does not forward settings absent from `compose.yaml`.

Enable backup synchronization separately on each server and configure its local
and remote retention policies. Inspect synchronization errors before assuming an
archive has an off-host copy. Failed synchronization retains the local archive.
The panel and Sparks must be able to reach and trust the storage endpoint for
upload and restore workflows.

Deleting a server also removes its local game data and backups. Keep a separate
recovery copy when retiring a server. Game snapshots do not replace backups of
the panel database and node identities; see [installation backups and upgrades](../README.md#backups-and-upgrades).
