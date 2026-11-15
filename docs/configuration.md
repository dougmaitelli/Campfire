---
title: Settings reference
---

Every panel setting below supports both an environment variable and a CLI flag.
Precedence is explicit CLI flag, then environment value, then built-in default.
For boolean flags, use `--flag=true` or `--flag=false` (a bare flag means `true`).
Deployment files may set values different from the binary defaults listed here. See the
[panel environment example](https://github.com/dougmaitelli/Campfire/blob/master/config/campfire.env.example)
for a configuration template.

## Core panel settings

| Setting               | CLI flag       | Purpose                                                         |
| --------------------- | -------------- | --------------------------------------------------------------- |
| `CAMPFIRE_PUBLIC_URL` | `--public-url` | Browser-visible HTTPS origin                                    |
| `CAMPFIRE_LISTEN`     | `--listen`     | Browser and API listen address; binary default `127.0.0.1:8080` |
| `CAMPFIRE_DATABASE`   | `--database`   | SQLite file path or PostgreSQL connection URL                   |
| `CAMPFIRE_DATA`       | `--data-dir`   | Persistent panel identities, certificates, and SFTP host key    |

SQLite is the default. To use PostgreSQL, create a dedicated database and set
`CAMPFIRE_DATABASE` to its connection URL. Keep `CAMPFIRE_DATA` persistent with
either backend: the database does not contain the panel's private signing
authority. Run one active panel process per installation.

## Spark connections

These settings configure the panel's listener for Spark agents, not browser access.

| Setting                     | CLI flag             | Purpose                                                                                                                     |
| --------------------------- | -------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `CAMPFIRE_AGENT_LISTEN`     | `--agent-listen`     | Dedicated Spark HTTPS listen address; defaults to `0.0.0.0:8443`; an empty value disables it                                |
| `CAMPFIRE_AGENT_PUBLIC_URL` | `--agent-public-url` | Spark connection URL when different from the default hostname and port                                                      |
| `CAMPFIRE_AGENT_TLS_CERT`   | `--agent-tls-cert`   | Custom server certificate path; provide together with the private key instead of using automatically generated certificates |
| `CAMPFIRE_AGENT_TLS_KEY`    | `--agent-tls-key`    | Private key for the custom Spark-listener certificate                                                                       |

## Authentication

| Setting                         | CLI flag                 | Purpose                                                                                       |
| ------------------------------- | ------------------------ | --------------------------------------------------------------------------------------------- |
| `CAMPFIRE_OIDC_ISSUER`          | `--oidc-issuer`          | Optional OpenID Connect provider issuer URL                                                   |
| `CAMPFIRE_OIDC_CLIENT_ID`       | `--oidc-client-id`       | OpenID Connect client ID                                                                      |
| `CAMPFIRE_OIDC_CLIENT_SECRET`   | `--oidc-client-secret`   | OpenID Connect client secret                                                                  |
| `CAMPFIRE_OIDC_AUTO_LINK_EMAIL` | `--oidc-auto-link-email` | Link first-time OIDC logins to local password accounts by verified email; defaults to `false` |
| `CAMPFIRE_DISABLE_LOCAL_LOGIN`  | `--disable-local-login`  | Require OIDC after an administrator has been linked; defaults to `false`                      |

See [accounts and access](/Campfire/authentication/) for provider setup and login safeguards.

## File access and uploads

| Setting                   | CLI flag           | Purpose                                                                               |
| ------------------------- | ------------------ | ------------------------------------------------------------------------------------- |
| `CAMPFIRE_MAX_UPLOAD_MIB` | `--max-upload-mib` | Per-file browser and SFTP upload limit; defaults to `1024` MiB; accepts `1–102400`    |
| `CAMPFIRE_SFTP_LISTEN`    | `--sftp-listen`    | Enable the optional SFTP listener; publish its TCP port separately                    |
| `CAMPFIRE_SFTP_HOST_KEY`  | `--sftp-host-key`  | Persistent SFTP SSH host-key path; defaults to `sftp_host_key` inside `CAMPFIRE_DATA` |

## Off-host backups

| Setting                  | CLI flag          | Purpose                                                                    |
| ------------------------ | ----------------- | -------------------------------------------------------------------------- |
| `CAMPFIRE_S3_BUCKET`     | `--s3-bucket`     | Optional S3 bucket for off-host backup storage                             |
| `CAMPFIRE_S3_REGION`     | `--s3-region`     | S3 region; defaults to `us-east-1`                                         |
| `CAMPFIRE_S3_PREFIX`     | `--s3-prefix`     | Optional object-key prefix for S3 backups                                  |
| `CAMPFIRE_S3_ENDPOINT`   | `--s3-endpoint`   | Custom endpoint for S3-compatible storage                                  |
| `CAMPFIRE_S3_PATH_STYLE` | `--s3-path-style` | Use path-style S3 addressing for compatible providers; defaults to `false` |

S3 credentials use the AWS SDK credential chain, including `AWS_ACCESS_KEY_ID`,
`AWS_SECRET_ACCESS_KEY`, and optional `AWS_SESSION_TOKEN`, shared configuration,
or an instance role. Credentials stay on the panel. Enable backup synchronization
and retention separately for each server; see [game backups](/Campfire/server-backups/).

## Monitoring and alerts

| Setting                              | CLI flag                      | Purpose                                                                                       |
| ------------------------------------ | ----------------------------- | --------------------------------------------------------------------------------------------- |
| `CAMPFIRE_RESOURCE_INTERVAL_SECONDS` | `--resource-interval-seconds` | Resource collection and alert evaluation interval; defaults to `10` seconds                   |
| `CAMPFIRE_RESOURCE_RETENTION_DAYS`   | `--resource-retention-days`   | Resource-history retention in days; defaults to `30`; accepts `1–365`                         |
| `CAMPFIRE_ALERT_CPU_PERCENT`         | `--alert-cpu-percent`         | CPU alert threshold relative to the server allocation; defaults to `90`, or `0` to disable    |
| `CAMPFIRE_ALERT_MEMORY_PERCENT`      | `--alert-memory-percent`      | Memory alert threshold relative to the server allocation; defaults to `90`, or `0` to disable |
| `CAMPFIRE_ALERT_DISK_PERCENT`        | `--alert-disk-percent`        | Disk alert threshold relative to the server allocation; defaults to `90`, or `0` to disable   |
| `CAMPFIRE_ALERT_DURATION_SECONDS`    | `--alert-duration-seconds`    | How long an alert or recovery condition must persist; defaults to `120` seconds               |
| `CAMPFIRE_ALERT_HEALTH_ENABLED`      | `--alert-health-enabled`      | Enable Spark connection and server health alerts; defaults to `true`                          |

### External notification delivery

In-app notifications work without Apprise. These optional settings enable external
delivery; URLs may contain credentials. See [notifications](/Campfire/notifications/).

| Setting                 | CLI flag         | Purpose                                         |
| ----------------------- | ---------------- | ----------------------------------------------- |
| `CAMPFIRE_APPRISE_URL`  | `--apprise-url`  | Complete Apprise notification endpoint          |
| `CAMPFIRE_APPRISE_TAGS` | `--apprise-tags` | Comma-separated Apprise tags                    |
| `CAMPFIRE_APPRISE_URLS` | `--apprise-urls` | Comma-separated inline notification target URLs |

## Activity and game-event retention

| Setting                              | CLI flag                      | Purpose                                                                                 |
| ------------------------------------ | ----------------------------- | --------------------------------------------------------------------------------------- |
| `CAMPFIRE_AUDIT_RETENTION_DAYS`      | `--audit-retention-days`      | Audit-history retention in days; defaults to `90`; `0` keeps indefinitely               |
| `CAMPFIRE_OPERATION_RETENTION_DAYS`  | `--operation-retention-days`  | Completed operation-history retention in days; defaults to `90`; `0` keeps indefinitely |
| `CAMPFIRE_GAME_EVENT_RETENTION_DAYS` | `--game-event-retention-days` | Game-event retention in days; defaults to `30`; accepts `1–3650`                        |
| `CAMPFIRE_GAME_EVENT_LIMIT`          | `--game-event-limit`          | Maximum retained game events per server; defaults to `10000`; accepts `1–100000`        |

Records older than the configured retention period are eligible for automatic
deletion. To retain audit and completed operation history indefinitely, explicitly
set both `CAMPFIRE_AUDIT_RETENTION_DAYS=0` and
`CAMPFIRE_OPERATION_RETENTION_DAYS=0`.

## Spark agent settings

These settings apply to the Spark process, not the panel. See
[adding a Spark](/Campfire/spark-agent/) for enrollment instructions and the
[Spark environment example](https://github.com/dougmaitelli/Campfire/blob/master/config/spark.env.example).

| Setting                  | CLI flag       | Purpose                                                                                                              |
| ------------------------ | -------------- | -------------------------------------------------------------------------------------------------------------------- |
| `SPARK_PANEL`            | `--panel`      | Panel's HTTPS agent URL for first enrollment                                                                         |
| `SPARK_DATA`             | `--data`       | Persistent private agent-state directory; binary default is `spark-data`                                             |
| `SPARK_CA_FILE`          | `--ca-file`    | Private CA certificate used to trust the panel's agent HTTPS listener                                                |
| `SPARK_TOKEN_FILE`       | `--token-file` | Private file containing a one-time enrollment token                                                                  |
| `SPARK_ENROLLMENT_TOKEN` | —              | One-time enrollment token supplied through the environment; do not combine with `SPARK_TOKEN_FILE` or `--token-file` |

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

## Applying configuration

For Compose, update the service's environment settings or the `.env` values it
references, then run `docker compose up -d` to recreate the service. For systemd,
edit `/etc/campfire/campfire.env` and restart `campfire`.

Changing `CAMPFIRE_DATABASE` selects a different database; it does not migrate
existing records between SQLite and PostgreSQL. Use the
[operations guide](/Campfire/operations/) to back up state before changes.
