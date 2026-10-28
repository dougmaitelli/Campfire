---
title: Settings reference
---

Command-line flags override environment values. The configuration examples list the available settings; the main deployment choices are:

| Setting                                                                          | Purpose                                                                                       |
| -------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `CAMPFIRE_PUBLIC_URL`                                                            | Browser-visible HTTPS origin                                                                  |
| `CAMPFIRE_DATABASE`                                                              | SQLite file path or PostgreSQL connection URL                                                 |
| `CAMPFIRE_DATA`                                                                  | Persistent panel identities, certificates, and SFTP host key                                  |
| `CAMPFIRE_AGENT_PUBLIC_URL`                                                      | Spark connection URL when different from the default hostname and port                        |
| `CAMPFIRE_OIDC_ISSUER`, `CAMPFIRE_OIDC_CLIENT_ID`, `CAMPFIRE_OIDC_CLIENT_SECRET` | Optional OpenID Connect provider                                                              |
| `CAMPFIRE_DISABLE_LOCAL_LOGIN`                                                   | Require OIDC after an administrator has been linked                                           |
| `CAMPFIRE_SFTP_LISTEN`                                                           | Enable the optional SFTP listener; publish its TCP port separately                            |
| `CAMPFIRE_MAX_UPLOAD_MIB`                                                        | Per-file browser and SFTP upload limit; defaults to 1024 MiB                                  |
| `CAMPFIRE_RESOURCE_INTERVAL_SECONDS`                                             | Resource collection and alert evaluation interval; defaults to `10` seconds                   |
| `CAMPFIRE_S3_BUCKET`, `CAMPFIRE_S3_REGION`, `CAMPFIRE_S3_ENDPOINT`               | Optional off-host backup storage                                                              |
| `CAMPFIRE_ALERT_CPU_PERCENT`                                                     | CPU alert threshold relative to the server allocation; defaults to `90`, or `0` to disable    |
| `CAMPFIRE_ALERT_MEMORY_PERCENT`                                                  | Memory alert threshold relative to the server allocation; defaults to `90`, or `0` to disable |
| `CAMPFIRE_ALERT_DISK_PERCENT`                                                    | Disk alert threshold relative to the server allocation; defaults to `90`, or `0` to disable   |
| `CAMPFIRE_ALERT_DURATION_SECONDS`                                                | How long an alert or recovery condition must persist; defaults to `120` seconds               |
| `CAMPFIRE_ALERT_HEALTH_ENABLED`                                                  | Enable Spark connection and server health alerts; defaults to `true`                          |
| `CAMPFIRE_APPRISE_URL`, `CAMPFIRE_APPRISE_TAGS`, `CAMPFIRE_APPRISE_URLS`         | Optional external notification delivery                                                       |
| `CAMPFIRE_AUDIT_RETENTION_DAYS`, `CAMPFIRE_OPERATION_RETENTION_DAYS`             | Independent history retention; `0` keeps records indefinitely                                 |

The [full panel environment example](https://github.com/dougmaitelli/Campfire/blob/master/config/campfire.env.example) lists additional settings. S3 credentials use the AWS SDK credential chain and stay on the panel; enable backup synchronization and retention separately for each server.

SQLite is the default. To use PostgreSQL, create a dedicated database and set `CAMPFIRE_DATABASE` to its connection URL. Keep `CAMPFIRE_DATA` persistent with either backend: the database does not contain the panel's private signing authority. Run one active panel process per installation.

## Optional Compose settings

The base Compose example uses the container image's defaults and only forwards
`CAMPFIRE_PUBLIC_URL` from `.env`. Add optional settings to the service's
`environment` section, either in `compose.yaml` or in a `compose.override.yaml`
beside it. Compose automatically merges that override when you run
`docker compose up -d`.

For example, this `compose.override.yaml` changes the upload limit:

```yaml
services:
  campfire:
    environment:
      CAMPFIRE_MAX_UPLOAD_MIB: "2048"
```

Adding a value to `.env` alone does not pass it into the container. Reference it
in the service's `environment` section when you want Compose to read it from
`.env`. Optional listeners such as SFTP also need a corresponding `ports` entry.

See [accounts and access](/Campfire/authentication/) for OIDC,
[game backups](/Campfire/server-backups/) for S3 storage, and
[notifications](/Campfire/notifications/) for alerts and Apprise.

## Applying configuration

For Compose, update the service's environment settings or the `.env` values it
references, then run `docker compose up -d` to recreate the service. For systemd,
edit `/etc/campfire/campfire.env` and restart `campfire`.

Changing `CAMPFIRE_DATABASE` selects a different database; it does not migrate
existing records between SQLite and PostgreSQL. Use the
[operations guide](/Campfire/operations/) to back up state before changes.
