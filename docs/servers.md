---
title: Servers
description: Operate servers, use the console, edit configuration, and apply Ember updates.
---

## Open and operate a server

Server rows in **Overview** and **Servers** open the console directly. Both lists
provide Start, Stop, and Restart controls. If you cannot read the console, the
server page shows the first tab you can access instead.

Power controls require `server.operate`. Updating or reinstalling game files
requires `server.reinstall`, not `server.configure`. A running process does not
necessarily mean the game is ready for players; check its output during startup.

## Console

The console shows installer output during installation and the game's standard
output and error streams while running. Reading output requires
`server.console.read`; sending commands requires `server.console.write`. A Log
viewer can follow output without power controls or command access. See the
[permission hierarchy](/Campfire/authentication/#permission-hierarchy).

CPU and memory graphs show measurements from the last hour; gaps indicate
unavailable readings. Resource bars show current usage, including game-data disk
usage. Disk usage has no history graph.

### Send a command

Enter a game command and choose **Send command**. Commands go to the game's
console input, not a shell on the Spark host. Use the syntax supported by the game.

### Logs and reconnection

The console loads recent output and follows new lines while connected. Reopening
or reconnecting refreshes recent output; it is not a permanent log archive. Use
the game's own log files for older records.

If output stops, check Spark connectivity and server status. Reverse proxies must
allow streaming responses without buffering. Use the **Files** and **Backups**
tabs for [game files](/Campfire/server-files/) and [snapshots](/Campfire/server-backups/).

## Respond to console alerts

Embers can define actionable error rules. Detected issues appear under **Needs
attention**, including after a process exits, and generate a notification. Follow
the guidance to open the relevant file or settings; these shortcuts do not change
configuration or run commands for you.

Console excerpts require console-read access, and file/settings shortcuts require
the corresponding permissions. Dismissing an issue requires `server.operate` and
dismisses that occurrence for all server users. A new execution can trigger the
same rule again; issues from earlier executions are archived.

See [Ember alert rules](/Campfire/ember-schema/#actionable-console-errors) to customize
detection.

## Edit a server

Open **More server actions** on the server page and choose **Edit server**.
This opens the existing editor directly and appears only when you can edit the
server. Management actions share this menu, with **Delete** last when permitted.
Resource links from other pages open a read-only details drawer without leaving
the current page.

Configure game settings in **Variables** and adjust resources and port mappings
as needed. To customize the imported startup command for this server, edit
**Startup command text** under **General**, or select **Use Ember default** to
reset it. Ember variable placeholders are supported; changes to the startup
command apply on the next server restart.

## Restart requirements

Configuration edits can be saved while the server runs. Spark receives the saved
settings without recreating the running container. **Restart required** appears
when the saved container settings differ from those currently running. Names,
descriptions, save-data paths, and log-monitoring rules do not require a restart.

## Apply an Ember update

When an imported Ember changes, **Ember update available** appears on the server
page and in the server lists, including for running servers. Choose **Review
Ember update** to apply the updated definition to the saved configuration. This
does not stop the server or run the installer. Save-data paths apply immediately;
container changes take effect on the next restart. The available-update and
pending-restart notices are independent, so both can appear if the Ember changes
again before a restart.

See [Ember definition updates](/Campfire/embers/#definition-updates)
for changes to the reusable game definition, and
[copies and transfers](/Campfire/server-transfers/) to duplicate a server or move
it to another Spark.
