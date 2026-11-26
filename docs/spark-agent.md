---
title: Sparks
---

Spark is the agent that runs game containers on a Linux game host. It needs a local Docker Engine
(rootful or rootless) and the Docker CLI. Campfire's guided installer also needs systemd, curl,
base64, and sudo access. Linux amd64 and arm64 packages are supported.

## Install and enroll

For a new host using rootless Docker, [prepare the game host](#use-rootless-docker)
before generating the time-limited enrollment command.

1. Create a Spark from **Sparks** in the panel.
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

### Use rootless Docker

Docker and game containers run under a dedicated, unprivileged Linux account.
Spark still runs as a root system service to enforce disk quotas and assign
ownership of installed files, uploads and restores. Run these steps on the
**game host** as an administrator. The examples use an account named `sparkdocker`.

1. **Prepare the account.** Install Docker Engine, its CLI, the rootless setup tool
   (`docker-ce-rootless-extras` with Docker's packages), `uidmap`, and `setfacl`
   (the `acl` package on Debian and Ubuntu). Use a filesystem with POSIX ACLs
   for Spark data. Create a regular user with a home directory if needed.

   ```sh
   sudo useradd --create-home sparkdocker
   grep '^sparkdocker:' /etc/subuid /etc/subgid
   ```

   Each file must assign the account at least 65,536 subordinate IDs. Follow
   [Docker's rootless installation instructions](https://docs.docker.com/engine/security/rootless/)
   for your distribution's packages and account prerequisites.

2. **Delegate resource limits.** Use a host with cgroup v2 and systemd. Before
   starting the new account's user service, run
   `sudo systemctl edit "user@$(id -u sparkdocker).service"` and save this drop-in.

   ```ini
   [Service]
   Delegate=cpu cpuset io memory pids
   ```

   On a **new host dedicated to rootless Docker**, the Engine package may have
   started the system daemon. Disable both its service and its activation socket
   with `sudo systemctl disable --now docker.service docker.socket`. On a host
   already running games, follow [Existing Sparks](#existing-sparks) before
   changing either service. To run both daemons, keep the system service and
   append `--force` to the rootless setup tool's `install` command.

   Reload systemd, enable the account's user services at boot, and install the
   daemon under that account.

   ```sh
   spark_uid=$(id -u sparkdocker)
   sudo systemctl daemon-reload
   sudo loginctl enable-linger sparkdocker
   sudo systemctl start "user@${spark_uid}.service"
   sudo -u sparkdocker -H env XDG_RUNTIME_DIR="/run/user/${spark_uid}" \
     DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/${spark_uid}/bus" \
     dockerd-rootless-setuptool.sh install
   ```

3. **Select and verify the socket.** Run the following in the administrator's
   shell. Copy the printed `DOCKER_HOST` line into `/etc/campfire/spark.env`,
   preserving other settings. Leave `DOCKER_CONTEXT` unset in Spark's environment.

   ```sh
   spark_socket="unix:///run/user/$(id -u sparkdocker)/docker.sock"
   printf 'DOCKER_HOST=%s\n' "$spark_socket"
   sudo mkdir -p /etc/campfire
   sudoedit /etc/campfire/spark.env
   sudo docker --host "$spark_socket" info
   ```

   Look for `rootless` under **Security Options**, `systemd` as the cgroup driver
   and `2` as the cgroup version. At startup Spark checks Docker can enforce CPU,
   memory, swap and PID limits. Choose game ports the account can bind, usually
   1024 and above. [Docker resource limits and port publishing](https://docs.docker.com/engine/security/rootless/tips/)

4. **Enroll and check a game server.** Use the usual [panel enrollment steps](#install-and-enroll).
   Before consuming the token, the installer checks the socket, resource limits,
   UID and GID mappings, and filesystem ACL support from the service environment. Once the panel
   shows the Spark connected, start a game server and confirm it appears on the
   selected daemon. Use the journal to diagnose startup failures.

   ```sh
   sudo docker --host "$spark_socket" ps --filter label=campfire.server
   sudo journalctl -u spark.service -b
   ```

Spark grants the daemon account directory traversal for its bind mounts. For a
custom `SPARK_DATA`, also grant the account traversal on parent directories
above that path. Restart Spark after changing Docker's UID or GID mappings.

#### Existing Sparks

- **Continuing with rootful Docker.** With `DOCKER_HOST` unset, Spark uses
  `/var/run/docker.sock` and its saved enrollment. The guided installer keeps
  `docker.service` as a boot dependency for the system daemon, as does the
  packaged systemd unit. For a custom socket, enable the service that owns it.
  If installing the unit manually for rootless Docker, remove its
  `Requires=docker.service` line. Rootful Docker with
  `userns-remap` is unsupported.
- **Switching an empty Spark.** Configure and check the rootless socket as above.
  Run `systemctl cat spark.service` to inspect the installed unit and drop-ins.
  If the unit or installer drop-in requires `docker.service`, or an old drop-in clears
  `EnvironmentFile=`, rerun the panel enrollment helper with a fresh token for
  **the same Spark**. This installs the updated unit and refreshes credentials.
  With the current binary and unit, `sudo systemctl restart spark.service` loads
  the socket setting using the saved identity.
- **Moving hosted games.** Each daemon has its own containers and images, and
  rootless Docker uses different host file owners. Keep the source Spark
  connected to its original daemon. Enroll a rootless Spark on a second host,
  then [transfer the servers](/Campfire/server-transfers/#move-a-server-to-another-spark)
  or [restore portable backups](/Campfire/server-backups/#portable-backups-and-recovery).

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

Optionally set **Default public host** to the IP or hostname players use to reach
this Spark. Do not include a scheme, path, or port. Unspecified addresses such as
`0.0.0.0` and `::`, and multicast addresses, cannot be used as this default. Servers without a **Public
game address** override inherit this host and their primary mapped host port.
Changing the default updates inherited addresses, but not explicit overrides.
This setting does not change networking or the agent connection URL.

Review existing reservations before reducing capacity or removing port allocations.

### Disk quotas and host storage

Spark probes the filesystem holding `SPARK_DATA/runtime` at startup. The host
setup determines how it enforces each server's game-data allocation.

| Filesystem       | Host setup and accounting                                                                                                                                                                                                                                                                                                                                                                                                                      |
| ---------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ext4 or XFS      | Use Linux 5.14+ with project quota accounting and enforcement. XFS accepts the `prjquota` or `pquota` mount option. ext4 also needs the project and quota filesystem features.                                                                                                                                                                                                                                                                 |
| Btrfs            | Install `btrfs-progs` and enable **full quota groups** on the filesystem. Wait for the quota rescan to finish. Spark creates a subvolume for each game-data directory and each staged restore, then limits its referenced bytes. The usage counter reflects committed extents, including extents shared with snapshots. [Btrfs quota groups](https://btrfs.readthedocs.io/en/latest/btrfs-qgroup.html)                                         |
| OpenZFS on Linux | Install the `zfs` CLI and use a dataset with project accounting available (`project_quota` pool feature). Spark sets `projectquota@ID` and reads `projectused@ID` on that dataset. ZFS enforces quotas asynchronously, so writes can briefly exceed the allocation. For rootless Docker, enable POSIX ACLs on the dataset. [OpenZFS quotas](https://openzfs.github.io/openzfs-docs/Basic%20Concepts/Datasets/Quotas%20and%20Reservations.html) |

Prepare the filesystem before starting Spark, and restart Spark after changing
quota support. If Spark starts during a full Btrfs quota rescan, it waits to
provision game data. Retry the operation once the rescan completes. Spark
checks the filesystem again without requiring a restart. On filesystems without
supported quotas, Spark monitors usage and cannot enforce a hard disk limit.

Spark prepares empty game-data directories before installation. To apply a
positive quota to populated, unmanaged game data, take a full backup, stop the
server and restore the backup. Spark prepares a separately limited staging
directory before replacing the original. An unlimited allocation allows an
existing ordinary directory to continue using logical-size accounting. Spark
preserves externally assigned quota projects and subvolumes.

**Backups on Btrfs.** Each managed `runtime/<server-id>/data` directory is its
own subvolume. A Btrfs snapshot of the parent contains an empty directory at
that path. It can preserve Spark's configuration and still omit every game save.
Use a full Campfire backup for each server or include each data subvolume in
your host snapshot job. See [host backup and recovery](/Campfire/operations/#btrfs-host-snapshots).

The panel identifies the accounting method and shows whether the configured
allocation is enforced. Managed directories retain native usage counters with
an unlimited allocation. Sparse files can consume less space than their logical
sizes suggest. Backup restores and Spark transfers recreate sparse holes, and
the filesystem enforces the allocation on file edits and uploads. Archive and
upload size limits still apply. On hosts without quota support and for unmanaged directories,
Spark counts logical file sizes using a bounded scan that skips symbolic links
and counts hard-linked files once.

If a managed counter fails or Spark cannot verify ownership, the last successful
reading stays visible with an error indicator. Spark caches readings for up to
30 seconds. The panel's collector refreshes them.

After an interrupted restore, Spark retries cleanup when it next receives a
request for that server. Its status shows any cleanup error. Other servers on
the same Spark can continue operating while you resolve the storage error.

Game-data allocations do not include backup archives, Docker images, or container
writable layers. Host storage therefore needs separate capacity management. Spark
usage totals combine its game servers, not the entire host; totals using different
accounting methods are labeled as mixed. Disk usage is current-only: retained
resource history and graphs intentionally cover CPU and memory, not disk.

## Crash diagnostics

Sparks advertising `server.crash-records.v1` observe Docker events independently
of panel connections and open browser sessions. Crash evidence is stored beside
private runtime state, not in game data or portable backups. The bounded replay
survives Spark restarts and is ingested into the panel's retained Activity history.
Older Sparks do not produce these records; existing retained records remain readable.

Docker's event history is bounded, so reconnect replay cannot guarantee recovery
of every exit during an outage. Monitoring or storage failures are reported on
the server page. Console capture failures preserve exit metadata and explicitly
mark the evidence unavailable. Spark does not introduce a restart loop or change
Docker's restart policy for this feature.

See [crash records](/Campfire/servers/#inspect-crash-records) for permissions,
retention limits, and OOM interpretation.

## Maintenance

| Action           | When to use it                                                      |
| ---------------- | ------------------------------------------------------------------- |
| **Update**       | Install the newer Spark build offered by the panel.                 |
| **Force update** | Reinstall the panel's bundled Spark build even when versions match. |
| **Re-enroll**    | Replace the Spark's credentials and restart its service.            |

Remote updates require a connected Spark supporting the selected action and use
the current panel's bundled package, not an independently selected release.
Updating preserves credentials, configuration, game files, and backups; game
containers keep running. Preserve the Spark data directory across service and
host restarts.

### Connection and feature troubleshooting

Inspect service logs with `sudo journalctl -u spark -f`. For connection problems,
check the configured agent address, outbound reachability, certificate trust,
and whether the Spark is enabled in the panel. An expired identity needs
re-enrollment. The **Features** view shows which operations a connected version supports.

Feature support is negotiated with the connected agent. A **Partial support**
entry means only some operations in that feature group are supported; it is not
confirmation that every related action is available. Update Spark when a required
capability is missing, then check the reported features again. If remote update
is unavailable, [upgrade the binary manually](/Campfire/operations/).

### Cleanup and removal

Use **Prune** to remove leftover Campfire installer containers, containers for
deleted servers, unused images tracked by Campfire, and unused volumes labeled
for Campfire. It preserves running and stopped game servers, their game files and
backups, and other applications' resources. It is not a general Docker system
prune. Finish active server operations and transfers before confirming cleanup,
and inspect the result for reclaimed resources.

Before restarting or updating Spark, let active installation, file, backup, and
transfer operations finish. Move or delete its servers before removing a Spark from the
panel, then stop or uninstall the service on the old host.
