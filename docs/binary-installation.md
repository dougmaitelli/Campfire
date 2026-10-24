---
title: Linux binary installation
---

Download the release archive for Linux amd64 or arm64 and its companion checksum file. From the download directory, replace `VERSION` and `ARCH` below with the downloaded values:

```sh
sha256sum -c campfire_VERSION_linux_ARCH.tar.gz.sha256
tar -xzf campfire_VERSION_linux_ARCH.tar.gz
sudo install -m 0755 campfire /usr/local/bin/campfire
```

The panel host needs operating-system CA certificates. Game hosts also need rootful Docker Engine and the Docker CLI; use the panel's enrollment helper to install Spark.

For a systemd panel installation, run these commands from the extracted archive:

```sh
sudo useradd --system --home-dir /var/lib/campfire --shell /usr/sbin/nologin campfire
sudo install -d -m 0750 /etc/campfire
sudo install -m 0600 campfire.env.example /etc/campfire/campfire.env
sudo install -m 0644 deploy/systemd/campfire.service /etc/systemd/system/
```

Edit `/etc/campfire/campfire.env` to set `CAMPFIRE_PUBLIC_URL` to your HTTPS origin. Configure the reverse proxy and agent connectivity as described in the Docker setup, then start the panel:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now campfire
sudo journalctl -u campfire -b
```

Use the setup token from the journal to create the first administrator. The service persists state in `/var/lib/campfire` and listens for browser traffic at `127.0.0.1:8080`. Continue with [adding a game host](/Campfire/spark-agent/).
