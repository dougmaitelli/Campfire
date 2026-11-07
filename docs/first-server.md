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

## Copy a server

Open a server's details drawer from **Servers**, then open its **Actions** menu.
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

Progress, retry, and cancellation appear in the server details drawer's **Jobs**
tab, alongside transfer history. Admins can also open **Jobs** in the sidebar to
review jobs across every server and filter by status or job type.
Reopen the server's Jobs tab after closing it or reloading
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
