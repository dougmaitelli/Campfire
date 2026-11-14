---
title: Your first server
---

First [install and enroll a Spark](/Campfire/spark-agent/) with a game address
players can reach. The panel itself does not run game containers.

1. Open **Embers** and import a native Ember or supported Pterodactyl/Pelican Egg. Review the image, installer, variables, and compatibility warnings.
2. Create a server and choose its Spark and Ember under **General**. Configure the game settings in **Variables**, then assign resources and ports. Edit **Startup command text** under **General** to customize the imported command for this server, or select **Use Ember default** to reset it. Ember variable placeholders are supported; later changes apply on the next server restart.
3. Install the server, then start it. Open its console to follow startup and send game commands.
4. Add server members, recurring schedules, and backups as needed.

An imported Egg is converted into a native Ember; successful conversion does not guarantee that its image or installer will run. The [compatibility guide](/Campfire/egg-compatibility/) explains what to review and how to handle import warnings.

Server rows in **Overview** and **Servers** open the console directly. Both lists
provide Start, Stop, and Restart controls. If you cannot read the console, the
server page shows the first tab you can access instead. Resource links from
other pages open a read-only details drawer without leaving the current page.

To change configuration, open **More server actions** on the server page and
choose **Edit server**. This opens the existing editor directly and appears only
when you can edit the server. Management actions share this menu, with **Delete**
last when permitted.

Configuration edits can be saved while the server runs. Spark receives the saved
settings without recreating the running container. **Restart required** appears
when the saved container settings differ from those currently running. Names,
descriptions, save-data paths, and log-monitoring rules do not require a restart.

When an imported Ember changes, **Ember update available** appears on the server
page and in the server lists, including for running servers. Choose **Review
Ember update** to apply the updated definition to the saved configuration. This
does not stop the server or run the installer. Save-data paths apply immediately;
container changes take effect on the next restart. The available-update and pending-restart notices
are independent, so both can appear if the Ember changes again before a restart.

## Move a server to another Spark

A transfer moves the existing server's game data and completed backups to another
Spark. Unlike a copy, it keeps the same server identity, ownership, access grants,
and schedules. Transferring requires both the platform `sparks.manage` and
`servers.manage_all` permissions; ordinary server operation access is not enough.

1. Apply any pending Ember update, finish active operations, and stop the server.
   A not-installed server can also be transferred. The source Spark must be
   reachable so Campfire can confirm the server is idle.
2. Open **More server actions → Transfer** and choose a different, enabled Spark.
   The destination needs capacity for the server and support for its required
   features. It can be disconnected when the job is queued; work waits for the
   required Sparks to connect.
3. Review each destination interface and host port against the displayed allowed
   ranges. Container ports and protocols stay unchanged. Ensure the new host's
   firewall allows the selected ports, and note the destination game address.
4. Choose **Transfer server**, then follow the job in **Activity → Operations**.
   Keep enough free space for archive staging and destination restoration. The
   server stays stopped throughout the transfer.
5. After completion, check its new placement and start it when ready. Players may
   need the new game address and host ports.

Closing the drawer or restarting the panel does not cancel the job. Failed jobs
retain their recovery state; inspect the error and retry rather than creating a
second transfer. Cancellation is available before placement is committed and
cleans up the unfinished destination while preserving the source. Cleanup may
wait for a disconnected Spark. After placement is committed, allow cleanup to
finish instead of trying to cancel the move.

## Copy a server

Open a server from **Servers**, then open **More server actions**.
Choose **Copy** to review the source settings in the creation wizard. Change the
name, destination Spark, resources, and port mappings as needed. **Copy game
files** is optional; a settings-only copy remains **Not installed** and can be
created while the source is running or its Spark is offline.

Choose **Quick copy** to select only a destination Spark and copy both settings
and game files. Campfire generates a unique name and assigns available host ports
while preserving the container ports. Both actions support the same Spark or a
different Spark. The caller owns the new server; an Admin can select another
owner through the Copy wizard.

File copies use a portable full backup and the existing upload and restore
operations. An installed source may be running or stopped. The source must be
online when submitting a file copy, but its enabled destination can be disconnected.
Copy and transfer jobs wait for their required Sparks to connect before executing;
waiting jobs do not consume worker slots. A running source stays running. The destination remains unavailable
until copying finishes, then stays stopped without running the installer.
Live copies have the same consistency behavior as [live backups](/Campfire/server-backups/):
they capture files as they are read and do not mirror subsequent changes.

Settings and backup policy are copied, including hidden variables handled by the
backend. Access grants, schedules, notifications, historical backups, activity,
and console history are not copied. Creating a copy requires server creation,
source configuration, and destination provisioning access; including files also
requires source file-read access.

Progress, retry, and cancellation appear in the server's **Activity** tab. Choose
**Operations** and open a copy or transfer to follow its progress.
Admins can also open **Jobs** in the sidebar to
review jobs across every server and filter by status or job type.
Reopen **Activity → Operations** after closing the details or reloading
the panel to follow the same copy. Failed copies retain their reservations and cannot start until
recovered or cancelled. Copy and transfer jobs use durable archive staging and
resume confirmed progress after panel or Spark restarts. Server configuration
and Ember definitions remain protected until the job finishes or cancellation
cleanup completes. A queued job can be cancelled without connecting either Spark.
Once work has started, cancellation removes the unfinished destination
and temporary archives; cleanup waits if a Spark is offline. The original server
and its existing backups are preserved. Allow temporary space for a full archive
on both Sparks, plus restoration staging on the destination. Existing backup
upload limits apply.
