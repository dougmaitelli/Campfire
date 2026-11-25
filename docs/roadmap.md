---
title: Roadmap
description: Planned improvements to Campfire.
---

Features we want to add to Campfire. This list is not a release schedule;
scope and order may change.

## Internationalization

- [ ] **Backend internationalization (i18n)** — Support localized user-facing
      messages from the backend, including errors and notifications.

## Notifications

- [ ] **PWA push notifications** — Receive server alerts through browser push
      notifications, including in the installed app, without keeping the panel open.

## Game console

- [ ] **Uptime graph** — Show the server's availability history alongside its
      current status in the game console.
- [ ] **Secondary and optional game-state fields** — Distinguish primary readings
      from secondary details and allow optional fields to be shown or hidden.

## Server lifecycle

- [ ] **Configurable restart policies** — Let administrators choose when servers
      restart automatically, including after crashes or unexpected exits.

## Spark infrastructure

- [ ] **Rootless Docker** — Support running Spark-managed game servers with a
      rootless Docker daemon.
- [ ] **Broader disk quota support** — Support enforcing server disk quotas on
      additional filesystems.

## Appearance

- [ ] **Theme system** — Support switching between visual themes across the panel.
- [ ] **Custom themes** — Allow users to create and apply their own themes on top
      of the theme system.
