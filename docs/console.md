---
title: Console
description: Follow live game output and send commands to a server.
---

Open a server's console to follow game output, inspect its resource usage, and
control its lifecycle. During installation, the console shows installer output.
Once the game is running, it follows the game's standard output and error streams.

CPU and memory graphs below the console show measurements from the last hour.
Gaps indicate unavailable measurements. The resource bars above the console show
current usage, including game-data disk usage; disk usage has no history graph.

## Send a command

Enter a game command in the command field and choose **Send command**. Commands
go to the game's console input; this is not a shell on the Spark host. Use the
command syntax supported by that game.

You need permission to view and operate the server. Starting, stopping, and
restarting are available through the server controls. A running process and a
game that is ready for players can be different states; check the logs when
startup takes longer than expected.

## Logs and reconnection

The console loads recent output and follows new lines while connected.
Reopening it or reconnecting refreshes the recent output; it is not a permanent
log archive. Use your game's own log files when you need older records.

If output stops, check the Spark connection and server state. For deployments
behind a reverse proxy, allow streaming responses without buffering.

Use the **Files** and **Backups** tabs for [game files](/Campfire/server-files/)
and [snapshots](/Campfire/server-backups/).
