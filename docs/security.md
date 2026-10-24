---
title: Security and storage
---

Campfire can control game processes and modify game data. Protect browser access with HTTPS and grant users only the permissions they need. Treat imported game images and installation scripts as executable code to review before running.

The supplied panel container runs as UID/GID `10001`, with a read-only root filesystem, dropped capabilities, and no Docker socket. Spark runs with the privileges needed to manage Docker and game file ownership. Keep its state on the Docker execution host, and protect both panel and Spark identity files.

Disk usage is measured on Spark. Hard game-data limits require Linux 5.14 or newer and XFS or ext4 with project quota accounting and enforcement enabled on the filesystem holding `SPARK_DATA/runtime`. XFS needs the `prjquota` or `pquota` mount option; ext4 also needs its project and quota filesystem features. Prepare the filesystem before starting Spark, and restart Spark after changing quota support. Without supported quotas, usage is monitored but hard disk limits are unavailable.

Game-data allocations do not include backup archives, Docker images, or container writable layers, so host storage still needs separate capacity management.
