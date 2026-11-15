---
title: Security and storage
---

Campfire can control game processes and modify game data. Protect browser access with HTTPS and grant users only the permissions they need. Treat imported game images and installation scripts as executable code to review before running.

The supplied panel container runs as UID/GID `10001`, with a read-only root filesystem, dropped capabilities, and no Docker socket. Spark runs with the privileges needed to manage Docker and game file ownership. Keep its state on the Docker execution host, and protect both panel and Spark identity files.

For filesystem requirements, hard game-data limits, and host capacity planning,
see [Spark disk quotas and host storage](/Campfire/spark-agent/#disk-quotas-and-host-storage).
