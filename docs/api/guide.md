---
title: Panel API integration guide
description: Read server status, track a start request, follow the live console, and page through audit records.
---

The examples use `campfire.example.org` as the panel host. Use an ID from `GET /api/v1/servers` in place of `example-survival`. See [Make your first request](/Campfire/api/#make-your-first-request) for token setup.

## Read server status

With `server.view`, request `GET /api/v1/servers/status` to read the current status of every visible server in one response. Each entry's `server_id` corresponds to an `id` in the server inventory. A server is ready for players when its `state` is `running` and `ready` is `true`. An `unknown` state means Campfire cannot confirm its availability.

For CPU and memory readings, `GET /api/v1/servers/example-survival/metrics/history?limit=24` returns up to 24 samples for that server. A null measurement indicates a gap in observation. For anonymous visitors, the [public status endpoint](/Campfire/api/publishing/#public-status-pages) exposes only the servers an administrator has published.

## Start a server and check the result

Starting an installed, stopped server requires `server.view` and `server.operate`. Send the following request when the server has no pending copy or transfer.

```http
POST /api/v1/servers/example-survival/actions HTTP/1.1
Host: campfire.example.org
Authorization: Bearer <token>
Content-Type: application/json
Content-Length: 18

{"action":"start"}
```

A `202 Accepted` response acknowledges dispatch. Read `GET /api/v1/servers/example-survival/status` to check the outcome. When the start response includes an `operation_id`, use it to identify that attempt. An `operation_state` of `failed` or `interrupted` signals failure. If a later operation has replaced it, the [server activity history](/Campfire/api/servers/#events-and-activity) records previous attempts.

## Follow live console output

The console stream requires `server.view` and `server.console.read`.

```http
GET /api/v1/servers/example-survival/console/stream HTTP/1.1
Host: campfire.example.org
Authorization: Bearer <token>
Accept: text/event-stream
```

The response stays open. Each server-sent event has a name and a JSON `data` field. An `output` event carries a fragment of console text.

```text
event: output
data: {"content":"Server started\n"}

```

If the stream ends, `GET /api/v1/servers/example-survival/console` returns a recent log snapshot. See the [console reference](/Campfire/api/servers/#console) for heartbeat, disconnect, and access-revocation events.

## Read audit history by page

The audit endpoint requires `audit.view` and returns events newest first. This request retrieves up to 100 events for one server.

```http
GET /api/v1/audit?target=server%3Aexample-survival&page=1&page_size=100 HTTP/1.1
Host: campfire.example.org
Authorization: Bearer <token>
```

The response contains `items` and `pagination`. If `pagination.total_pages` is greater than one, request `page=2` with the same filter and page size. The collection is live, so new events can shift page boundaries between requests. Each event has a stable `id` for recognizing the same record across pages.
