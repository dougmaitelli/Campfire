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

## Maintenance

Use **Re-enroll** in Spark details when replacing its credentials. Successful
re-enrollment replaces the previous identity and briefly restarts the service.
Preserve the Spark data directory across ordinary service and host restarts.

Inspect service logs with `sudo journalctl -u spark -f`. For connection problems,
check the configured agent address, outbound reachability, certificate trust,
and whether the Spark is enabled in the panel. An expired identity needs
re-enrollment. The **Features** view shows which operations a connected version supports.

Before restarting or updating Spark, let active installation, file, and backup
operations finish. Move or delete its servers before removing a Spark from the
panel, then stop or uninstall the service on the old host.
