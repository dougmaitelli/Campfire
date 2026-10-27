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
| `CAMPFIRE_RESOURCE_INTERVAL_SECONDS`                                             | Resource collection interval; defaults to `10` seconds                                        |
| `CAMPFIRE_S3_BUCKET`, `CAMPFIRE_S3_REGION`, `CAMPFIRE_S3_ENDPOINT`               | Optional off-host backup storage                                                              |
| `CAMPFIRE_ALERT_CPU_PERCENT`                                                     | CPU alert threshold relative to the server allocation; defaults to `90`, or `0` to disable    |
| `CAMPFIRE_ALERT_MEMORY_PERCENT`                                                  | Memory alert threshold relative to the server allocation; defaults to `90`, or `0` to disable |
| `CAMPFIRE_ALERT_DISK_PERCENT`                                                    | Disk alert threshold relative to the server allocation; defaults to `90`, or `0` to disable   |
| `CAMPFIRE_ALERT_DURATION_SECONDS`                                                | How long an alert or recovery condition must persist; defaults to `120` seconds               |
| `CAMPFIRE_ALERT_INTERVAL_SECONDS`                                                | Alert evaluation interval; defaults to `30` seconds                                           |
| `CAMPFIRE_ALERT_HEALTH_ENABLED`                                                  | Enable Spark connection and server health alerts; defaults to `true`                          |
| `CAMPFIRE_APPRISE_URL`, `CAMPFIRE_APPRISE_TAGS`, `CAMPFIRE_APPRISE_URLS`         | Optional external notification delivery                                                       |
| `CAMPFIRE_AUDIT_RETENTION_DAYS`, `CAMPFIRE_OPERATION_RETENTION_DAYS`             | Independent history retention; `0` keeps records indefinitely                                 |

The base Compose file forwards the settings shown in [`.env.example`](https://github.com/dougmaitelli/Campfire/blob/master/.env.example). For additional settings such as S3 storage, add the corresponding environment entries to your Compose service using the [full panel example](https://github.com/dougmaitelli/Campfire/blob/master/config/campfire.env.example). S3 credentials use the AWS SDK credential chain and stay on the panel; enable backup synchronization and retention separately for each server.

SQLite is the default. To use PostgreSQL, create a dedicated database and set `CAMPFIRE_DATABASE` to its connection URL. Keep `CAMPFIRE_DATA` persistent with either backend: the database does not contain the panel's private signing authority. Run one active panel process per installation.

## Applying configuration

For Compose, edit `.env` and run `docker compose up -d` to recreate the service
with the updated environment. For systemd, edit `/etc/campfire/campfire.env`
and restart `campfire`.

Changing `CAMPFIRE_DATABASE` selects a different database; it does not migrate
existing records between SQLite and PostgreSQL. Use the
[operations guide](/Campfire/operations/) to back up state before changes.
