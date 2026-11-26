---
title: Copies and transfers
description: Copy a game server or move it to another Spark, and follow job progress and recovery.
---

## Choose an action

| Action         | Result                                                         | What you choose                                                               |
| -------------- | -------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| **Copy**       | An independent server, with settings and optionally game files | Name, Spark, resources, ports, and whether to include files                   |
| **Quick copy** | An independent server with settings and game files             | Destination Spark; Campfire generates a name and assigns available host ports |
| **Transfer**   | The existing server moved to another Spark                     | Destination Spark and host port mappings                                      |

Copies can use the same Spark or a different one. Transfers preserve the existing
server identity, ownership, access grants, schedules, and completed backups.
Copies preserve settings and backup policy, including hidden variable values, but
not access grants, schedules, notifications, historical backups, activity, or
console history.

## Copy a server

Open a server from **Servers**, then choose **More server actions → Copy** or
**Quick copy**. The caller owns the new server; an Admin can select another owner
through the Copy wizard. Creating a copy requires server creation, source
configuration, and destination provisioning access. Including files also requires
source file-read access.

A settings-only copy remains **Not installed** and can be created while the source
is running or its Spark is offline.

For a file copy, the source must be installed and online when submitted, but may
be running or stopped. A running source stays running. The destination remains
unavailable until copying finishes, then stays stopped without running the
installer. Container ports are preserved; review the assigned host ports.

Live copies have the same consistency behavior as
[live backups](/Campfire/server-backups/#create-a-backup): they capture files as
they are read and do not mirror subsequent changes.

## Move a server to another Spark

Transferring requires both platform `sparks.manage` and `servers.manage_all`
permissions; ordinary server operation access is not enough.

1. Apply any pending Ember update, finish active operations, and stop the server.
   A not-installed server can also be transferred. The source Spark must be
   reachable so Campfire can confirm the server is idle.
2. Open **More server actions → Transfer** and choose a different, enabled Spark.
   It needs capacity and support for the server's required features.
3. Review destination interfaces and host ports against the allowed ranges.
   Container ports and protocols stay unchanged. Ensure the destination firewall
   allows the selected ports.
4. Choose **Transfer server**. The server stays stopped throughout the transfer.
5. After completion, check its placement and start it when ready. The server's
   explicit **Public game address** override is preserved; an inherited address
   follows the destination Spark's default public host and mapped primary port.
   Review the connection address in the server editor before sharing it.

## Progress and recovery

File-copy and transfer jobs can be queued for an enabled but disconnected
destination. They wait for their required Sparks to connect before executing.
Allow temporary space for a full archive on both Sparks, plus restoration staging
on the destination. File copies use portable full backups and are subject to the
backup upload limits.

Open **Activity → Operations** and select the job to follow progress, retry, or
cancel. Admins can use **Jobs** for a cross-server view. Closing the drawer or
restarting the panel does not cancel a job; reopen Activity to follow it again.
Jobs retain recovery state and resume confirmed progress after panel or Spark
restarts. Inspect an error and retry the existing job rather than starting another.

Failed copies retain their reservations and cannot start until recovered or
cancelled. Server configuration and Ember definitions remain protected until the
job finishes or cancellation cleanup completes.

### Cancellation

A queued job can be cancelled without connecting either Spark. Once work starts,
cancellation removes the unfinished destination and temporary archives, preserving
the source server and its backups. Cleanup waits if a required Spark is offline.

For transfers, cancellation is available only before placement is committed.
After commitment, allow cleanup to finish instead of trying to cancel the move.
