---
title: How Campfire works
description: Understand the relationship between the Campfire panel, Spark game hosts, Ember game definitions, and game servers.
---

Campfire manages games through a central panel, agents on your game hosts, and
reusable game definitions. The names below appear in the interface and throughout
these guides.

| Name         | Common term      | Purpose                                              |
| ------------ | ---------------- | ---------------------------------------------------- |
| **Campfire** | Management panel | The web interface where you manage your games        |
| **Spark**    | Game host agent  | Runs game containers on a Linux machine              |
| **Ember**    | Game definition  | Describes how to install, configure, and run a game  |
| **Server**   | Game instance    | One game installation with its own settings and data |

## Campfire: the panel

Campfire is the application you open in your browser. It manages accounts,
permissions, game configuration, and schedules, and coordinates operations with
your Sparks.

The panel does not run game containers itself. When you start a server or send a
console command, Campfire asks the Spark hosting that server to carry out the
operation.

## Sparks: the game hosts

A Spark is the agent installed on a Linux machine that hosts your games. In the
interface, each Spark represents that host and its connection to the panel.

Spark manages the Docker containers, game files, console output, local backups,
and resource measurements on its host. **One Spark can host multiple servers**,
and one Campfire panel can manage multiple Sparks.

You can run the panel and a Spark on the same machine, or use separate machines.
Even when they share a machine, you still need Spark to run the games.

See [adding a Spark](/Campfire/spark-agent/) for installation and enrollment.

## Embers: the game definitions

An Ember describes how a game is installed and started: its container images,
installation steps, startup command, variables, and configuration rules.

Think of it as a reusable template. **One Ember can be used by multiple servers**,
including servers on different Sparks. Importing an Ember adds the definition to
Campfire; creating and installing a server from it is a separate step.

You can import native Embers or convert supported Pterodactyl and Pelican Eggs.
See [importing game definitions](/Campfire/egg-compatibility/).

## Servers: the games you run

A server is a particular game instance, such as a Minecraft survival world. It
has its own name, game files, settings, ports, resource allocation, and access
permissions. Here, “server” means the game instance rather than the entire Linux
machine.

When you create a server, you choose:

- **A Spark:** where the game will run.
- **An Ember:** how the game will be installed and started.

Two servers can share a Spark and an Ember while keeping separate worlds,
settings, and game ports. Players connect directly to the game ports on the
hosting Spark, while you manage the servers through Campfire.

## An example: two Minecraft worlds

Suppose you want a survival world and a creative world for your friends:

1. Install the **Campfire panel** and create your administrator account.
2. Connect a **Spark** named **Home** on the Linux machine that will run the games.
3. Import one **Minecraft Ember** that defines how to install and start Minecraft.
4. Create two **servers**, **Survival** and **Creative**, both using the Home Spark
   and the Minecraft Ember. Give each its own game port and settings.
5. Install and start both servers. Each has its own world and console, and you
   manage both from the same panel.

You now have **one Campfire panel, one Spark, one Ember, and two servers**. Adding
another game host means adding a Spark; adding another world means creating a
server.

## Choose how to install

Start with [Docker Compose](/Campfire/getting-started/) or a
[Linux binary](/Campfire/binary-installation/), then follow
[your first server](/Campfire/first-server/) to connect a Spark, import an Ember,
and create a game instance.
