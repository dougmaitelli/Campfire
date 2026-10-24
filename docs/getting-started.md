---
title: Docker Compose setup
---

Use the included [`compose.yaml`](https://github.com/dougmaitelli/Campfire/blob/master/compose.yaml) and [`.env.example`](https://github.com/dougmaitelli/Campfire/blob/master/.env.example), either from a release archive or a checkout:

```sh
git clone https://github.com/dougmaitelli/Campfire.git
cd Campfire
cp .env.example .env
```

Edit `.env` and set your panel's public URL:

```dotenv
CAMPFIRE_PUBLIC_URL=https://panel.example.com
```

Configure an HTTPS reverse proxy for your panel hostname, forwarding to `http://127.0.0.1:8080`. The supplied Compose file binds the browser port to loopback and separately publishes the Spark TLS listener on port `8443`.

```sh
docker compose pull
docker compose up -d
```

Open your configured HTTPS URL. On first startup, the logs contain a one-time administrator setup token. Enter it in the setup form, choose a username, and create a password of at least 15 characters. Restarting before setup completes generates a new token; use the latest startup output.

The panel generates and persists its Spark TLS certificates automatically. By default, agents connect to the panel hostname on port `8443`; set `CAMPFIRE_AGENT_PUBLIC_URL` if they need a different hostname or external port. This listener needs direct TLS or TCP passthrough, since Spark connections use client certificates. Configure your HTTP proxy to stream console responses without buffering.

To supply your own agent certificate, set `CAMPFIRE_TLS_DIR` to a directory containing `panel.crt` and `panel.key`, readable by container UID/GID `10001`. Start Compose with `-f compose.yaml -f compose.agent-tls.yaml`. Preserve the certificate authority trusted by enrolled Sparks when changing TLS settings.

Continue with [adding a Spark](/Campfire/spark-agent/) and [creating your first server](/Campfire/first-server/).
