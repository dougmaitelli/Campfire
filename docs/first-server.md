---
title: Your first server
description: Create, install, and start a game server, then connect to it as a player.
---

This guide takes you from an enrolled game host to a running game that players
can connect to.

## Before you begin

[Install the panel](/Campfire/getting-started/) and
[enroll a Spark](/Campfire/spark-agent/) with a game address players can reach.
The panel itself does not run game containers. Make sure the Spark is connected
and that your account can create servers on it.

## 1. Import a game definition

Open **Embers** and import a native Ember or supported Pterodactyl/Pelican Egg for
your game. Review the image, installer, variables, and any compatibility warnings.

An imported Egg is converted into a native Ember. Successful conversion does not
guarantee its image or installer will run; use the
[Ember compatibility guide](/Campfire/embers/) if you need help choosing
or reviewing a definition.

## 2. Create the server

Open **Servers** and create a server:

1. Choose its Spark and Ember under **General** and give it a recognizable name.
2. Configure the game settings in **Variables**.
3. Assign the resources the game needs and configure its port mappings. Use
   available host ports and allow them through the Spark host's firewall.
4. Finish creating the server.

Keep the Ember's startup command for the initial setup unless your game requires
an override. You can [edit the server configuration](/Campfire/servers/)
later.

## 3. Install and start

Install the server, then start it. Open its [console](/Campfire/servers/#console) to follow
installation and startup output. Wait for the game to report that it is ready
for players; a running process does not always mean startup has finished.

If installation or startup fails, check the console output and review the
Ember's requirements and variable values before retrying.

## 4. Connect to the game

Use the Spark's reachable game address and the server's assigned **host port**
in your game client, following the connection method supported by that game.
The panel's browser URL is not the game connection address.

If you cannot connect, confirm that the game is ready in the console, the host
port mapping is correct, and the host firewall and any router port forwarding
allow the game's required ports and protocols.

## Next steps

- [Create backups](/Campfire/server-backups/) before making important changes.
- [Share access](/Campfire/authentication/#roles-and-server-membership) with other users.
- [Schedule recurring actions](/Campfire/schedules/).
- [Edit configuration and apply Ember updates](/Campfire/servers/).
- [Copy or transfer a server](/Campfire/server-transfers/) when you need another instance or host.
