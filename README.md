<p align="center">
  <img src="web/public/favicon.svg" alt="Campfire" width="96">
</p>

<h1 align="center">Campfire</h1>

[![CI](https://github.com/dougmaitelli/Campfire/actions/workflows/ci.yml/badge.svg)](https://github.com/dougmaitelli/Campfire/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/dougmaitelli/Campfire)](https://github.com/dougmaitelli/Campfire/releases)
[![GHCR](https://img.shields.io/badge/container-ghcr.io-blue)](https://github.com/dougmaitelli/Campfire/pkgs/container/campfire)

Campfire is a self-hosted game server management panel. Run games across Linux hosts, stream their consoles, edit their files, manage backups, and give friends access to the servers they use—all from one web interface.

The panel manages accounts, permissions, configuration, and schedules. A separate **Spark** agent on each game host runs the Docker containers and connects back to the panel over authenticated TLS. Game definitions, called **Embers**, describe how to install, configure, and run each game; you can import native definitions or supported Pterodactyl and Pelican Eggs.

## Documentation

- [Documentation website](https://dougmaitelli.github.io/Campfire/)
- [Configuration reference](https://dougmaitelli.github.io/Campfire/configuration/)
- [Embers and Egg imports](https://dougmaitelli.github.io/Campfire/egg-compatibility/)
- [Development](https://dougmaitelli.github.io/Campfire/development/)

## Features

- **Multiple game hosts** — enroll Sparks, track their connection status, and allocate CPU, memory, disk, and game ports to servers
- **Game server controls** — install, start, stop, restart, and inspect background operations from the browser
- **Live console** — stream game and installer output and send commands to a running server
- **File management** — browse game files, edit text, upload and download files, and compress or extract archives
- **Optional SFTP gateway** — access game files through the panel using a personal API token and the same server permissions
- **Backups and restoration** — create and restore local snapshots, with optional S3-compatible off-host storage and per-server retention
- **Recurring schedules** — configure future server actions with execution history separate from direct operations
- **Ember imports and updates** — preview native definitions or supported Egg conversions, review warnings, and explicitly apply definition updates
- **Users and roles** — combine platform roles, server ownership, and server memberships to control access; inspect linked resources in drawers without leaving the current page
- **Authentication and API tokens** — local password accounts, optional OpenID Connect, and scoped, expiring personal tokens
- **Monitoring and notifications** — view live resource usage, receive sustained resource and health alerts, and optionally deliver notifications through Apprise
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
```

Open your HTTPS URL and create the first administrator using the one-time setup token in the startup logs. Then add a Spark using its generated installation command, import an Ember, and create your first game server.

See [Docker setup](https://dougmaitelli.github.io/Campfire/getting-started/), [binary installation](https://dougmaitelli.github.io/Campfire/binary-installation/), and [your first server](https://dougmaitelli.github.io/Campfire/first-server/) for the full steps.

## Authentication

Administrators create password accounts and manage built-in or custom roles. Server ownership and memberships control access to individual servers. Optional OpenID Connect supports SSO, and scoped personal API tokens support API and SFTP access.

See [accounts and access](https://dougmaitelli.github.io/Campfire/authentication/) for OIDC setup and permissions, and [operations](https://dougmaitelli.github.io/Campfire/operations/) for password recovery and upgrades.

## Support

Use [GitHub issues](https://github.com/dougmaitelli/Campfire/issues) for reproducible bugs and feature requests. Include the Campfire and Spark versions, relevant deployment details, and logs with credentials and enrollment tokens removed.
