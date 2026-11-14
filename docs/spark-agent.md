---
title: Game hosts
---

A Spark is a Linux host that runs game containers. It needs rootful Docker Engine
and the Docker CLI. Campfire's guided installer also needs systemd, curl,
base64, and sudo access. Linux amd64 and arm64 packages are supported.

## Install and enroll

1. Create a Spark from **Sparks** in the panel. Use a game address players can reach.
2. Open its enrollment helper and check the agent connection URL.
3. Run the generated command on the game host. It installs Spark and its systemd
   service and saves the enrolled identity in `/var/lib/spark`.
4. Confirm that the Spark is connected, then open the game ports needed by players.

The command contains a single-use token valid for 15 minutes. Generate another
if it expires. Generated panel certificates are handled by the helper; custom
private certificates may require a CA file on the host.

Spark connects outbound to the panel's agent TLS listener, normally port `8443`.
It needs no inbound management port. Game traffic uses the host's allocated game
ports, separately from the management connection.

## Capacity and networking

Edit a Spark's **Resources & networking** settings to define the capacity
available to game servers and the host port ranges they may reserve. CPU, memory,
and disk limits are allocation budgets; set a limit to `0` for no configured
limit. Leave enough capacity for the host operating system, Docker, backup
archives, and temporary restoration files rather than allocating the whole host.

Port allocations specify a bind address, TCP or UDP, and a port or range. Each
server reserves host ports from these allocations while its Ember defines the
container ports. Configure both protocols when the game requires them. Allocation
does not open a firewall or configure router forwarding: make the selected host
ports reachable separately.

The Spark's game address is what players use to reach its servers; it is distinct
from the panel's agent connection URL and the interface Docker binds. Check it
when moving servers between hosts. Review existing reservations before reducing
capacity or removing port allocations.

## Maintenance

Use **Update** when Campfire offers a newer bundled Spark build. Remote updates
require a connected agent with update support; clients without that capability
need a manual upgrade. Updates install the package supplied by the current panel,
not an independently selected release.

Use **Re-enroll** in Spark details when replacing its credentials. Successful
re-enrollment replaces the previous identity and briefly restarts the service.
Preserve the Spark data directory across ordinary service and host restarts.

Use **Force update** in Spark details to install the client build bundled with the
current panel remotely, even when no update is detected or the installed version
matches the panel. This requires a connected Spark with remote force update
support. Confirm the action to download and verify the panel's package, replace
the executable, and restart Spark. Credentials, configuration, game files, and
backups are preserved; game containers keep running. Older clients need an
ordinary update before they can support this action.

Inspect service logs with `sudo journalctl -u spark -f`. For connection problems,
check the configured agent address, outbound reachability, certificate trust,
and whether the Spark is enabled in the panel. An expired identity needs
re-enrollment. The **Features** view shows which operations a connected version supports.

Feature support is negotiated with the connected agent. A **Partial support**
entry means only some operations in that feature group are supported; it is not
confirmation that every related action is available. Update Spark when a required
capability is missing, then check the reported features again.

Use **Prune** to remove leftover Campfire installer containers, containers for
deleted servers, unused images tracked by Campfire, and unused volumes labeled
for Campfire. It preserves running and stopped game servers, their game files and
backups, and other applications' resources. It is not a general Docker system
prune. Finish active server operations and transfers before confirming cleanup,
and inspect the result for reclaimed resources.

Before restarting or updating Spark, let active installation, file, backup, and
transfer operations finish. Move or delete its servers before removing a Spark from the
panel, then stop or uninstall the service on the old host.
