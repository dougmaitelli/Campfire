<p align="center">
  <img src="web/public/favicon.svg" alt="Campfire" width="96">
</p>

<h1 align="center">Campfire</h1>

[![CI](https://github.com/dougmaitelli/Campfire/actions/workflows/ci.yml/badge.svg)](https://github.com/dougmaitelli/Campfire/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/dougmaitelli/Campfire)](https://github.com/dougmaitelli/Campfire/releases)
[![GHCR](https://img.shields.io/badge/container-ghcr.io-blue)](https://github.com/dougmaitelli/Campfire/pkgs/container/campfire)

Campfire is a self-hosted game server management panel. Run games across Linux hosts, stream their consoles, edit their files, manage backups, and give friends access to the servers they use—all from one web interface.

| Name         | Role                                                                  |
| ------------ | --------------------------------------------------------------------- |
| **Campfire** | The panel for accounts, permissions, configuration, and schedules     |
| **Spark**    | An agent on a Linux game host that runs the Docker containers         |
| **Ember**    | A reusable definition for installing, configuring, and running a game |
| **Server**   | One game instance with its own settings, files, ports, and resources  |

One Spark can host several servers, and multiple servers can share an Ember.
The panel and a Spark can run on the same machine; Spark is still needed to run
the games. See [how Campfire works](https://dougmaitelli.github.io/Campfire/concepts/)
for an example.

## Documentation

- [Documentation website](https://dougmaitelli.github.io/Campfire/)
- [Getting started and core concepts](https://dougmaitelli.github.io/Campfire/concepts/)
- [Configuration reference](https://dougmaitelli.github.io/Campfire/configuration/)
- [Embers and Egg imports](https://dougmaitelli.github.io/Campfire/egg-compatibility/)
- [Development](https://dougmaitelli.github.io/Campfire/development/)

## Features

- **Multiple game hosts** — enroll Sparks, track their connection status, and allocate CPU, memory, disk, and game ports to servers
- **Game server controls** — install, start, stop, restart, and inspect background operations from the browser
- **Console and resource history** — stream game and installer output, send commands, and follow CPU and memory graphs
- **File management** — browse game files, edit text, upload and download files, and compress or extract archives
- **Optional SFTP gateway** — access game files through the panel using a personal API token and the same server permissions
- **Backups and restoration** — choose full-server or Ember-defined save-data snapshots for manual and scheduled backups, with optional S3-compatible off-host storage and per-server retention
- **Recurring schedules** — configure future server actions with execution history separate from direct operations
- **Ember imports and updates** — preview native definitions or supported Egg conversions, review warnings, and explicitly apply definition updates
- **Users and roles** — combine platform roles, server ownership, and server memberships to control access; inspect linked resources in drawers without leaving the current page
- **Permission preview** — Admin-only, read-only “View as…” to inspect another user's access
- **Authentication and API tokens** — local password accounts, optional OpenID Connect, and scoped, expiring personal tokens
- **Monitoring and notifications** — view live resource usage, receive sustained resource and health alerts, and optionally deliver notifications through Apprise
- **Game integrations** — display player counts, game state, and retained events published by game mods or adapters
- **Audit history** — record administrative changes, authentication events, and operation outcomes with configurable retention
- **SQLite or PostgreSQL** — use an embedded database or an existing PostgreSQL service
- **Two deployment options** — run the panel in Docker or as a Linux binary; the production binary includes the web UI

## Screenshots

<table>
<tr>
<td width="50%">
<a href="screenshots/dashboard.png"><img src="screenshots/dashboard.png" alt="Campfire overview with resource allocations and server status"></a>
<p><em>Overview — resource usage, allocations, and server status across your Sparks.</em></p>
</td>
<td width="50%">
<a href="screenshots/server-console.png"><img src="screenshots/server-console.png" alt="Server console with live game logs and command input"></a>
<p><em>Console — live game output and command input alongside server details.</em></p>
</td>
</tr>
<tr>
<td width="50%">
<a href="screenshots/schedules.png"><img src="screenshots/schedules.png" alt="Recurring schedules and scheduled execution history"></a>
<p><em>Schedules — recurring server actions and their execution history.</em></p>
</td>
<td width="50%">
<a href="screenshots/server-backups.png"><img src="screenshots/server-backups.png" alt="Backups tab inside the server console"></a>
<p><em>Server backups — inspect snapshots and storage availability from the console's Backups tab.</em></p>
</td>
</tr>
</table>

## Quick start with Docker Compose

You need Docker Engine with Compose for the panel, and a Linux game host with
rootful Docker Engine and the Docker CLI for Spark. The guided Spark installer
also uses systemd, curl, base64, and sudo. See
[game host requirements](https://dougmaitelli.github.io/Campfire/spark-agent/)
for details.

Use the included [compose.yaml](compose.yaml) and [.env.example](.env.example):

```sh
git clone https://github.com/dougmaitelli/Campfire.git
cd Campfire
cp .env.example .env
```

Set `CAMPFIRE_PUBLIC_URL` in `.env` to your panel's HTTPS origin. Configure an HTTPS reverse proxy forwarding to `http://127.0.0.1:8080`, and allow game hosts to reach the separate Spark TLS listener on port `8443`.

```sh
docker compose pull
docker compose up -d
docker compose logs campfire
```

Open your HTTPS URL and create the first administrator using the one-time setup token in the startup logs. Then add a Spark using its generated installation command, import a native Ember or supported Pterodactyl/Pelican Egg, and create, install, and start your first game server.

See [Docker setup](https://dougmaitelli.github.io/Campfire/getting-started/), [binary installation](https://dougmaitelli.github.io/Campfire/binary-installation/), and [your first server](https://dougmaitelli.github.io/Campfire/first-server/) for the full steps.

## Configuration and recovery

The example Compose file uses SQLite and the container image's defaults. Only
the public URL is required in `.env`. Add optional settings for OIDC, S3 storage,
SFTP, or Apprise through the Compose service's environment; see the
[configuration guide](https://dougmaitelli.github.io/Campfire/configuration/#optional-compose-settings)
and [full environment reference](config/campfire.env.example). Values added to
`.env` are passed to the container only when referenced by the service.

Keep the `campfire-data` volume persistent: it holds private identities and the
default SQLite database. Back up the panel volume, your database, and each
Spark's state and game data before upgrades. Game snapshots protect individual
games; they do not replace an installation backup. Follow the
[upgrade and recovery guide](https://dougmaitelli.github.io/Campfire/operations/)
for backup, upgrade, and administrator recovery procedures.

## Authentication

Administrators create password accounts and manage built-in or custom roles. Server ownership and memberships control access to individual servers. Optional OpenID Connect supports SSO, and scoped personal API tokens support API and SFTP access.

See [accounts and access](https://dougmaitelli.github.io/Campfire/authentication/) for OIDC setup and permissions, and [operations](https://dougmaitelli.github.io/Campfire/operations/) for password recovery and upgrades.

## Development

With the [development prerequisites](https://dougmaitelli.github.io/Campfire/development/)
installed, start the app with simulated game hosts:

```sh
make dependencies
make dev-mock
```

Open `http://localhost:8081` and sign in with `admin` / `campfire-dev-password`.
Mock mode uses the real authentication, API, and database with sample games and
simulated Sparks. See the
[development guide](https://dougmaitelli.github.io/Campfire/development/) for tests
and local documentation commands.

## Support

Use [GitHub issues](https://github.com/dougmaitelli/Campfire/issues) for reproducible bugs and feature requests. Include the Campfire and Spark versions, relevant deployment details, and logs with credentials and enrollment tokens removed.
