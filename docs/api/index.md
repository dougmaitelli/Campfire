---
title: Panel API reference
description: Authenticate an external tool and make HTTP requests to your Campfire panel.
---

Use the panel's HTTP API to build a server dashboard, run scheduled work, or integrate Campfire with another service. The base URL is **your panel's origin followed by `/api/v1`**. The endpoint tables below use paths relative to that base. For mods that publish game state or events from inside a game container, see [Game integrations](/Campfire/game-integrations/).

## Make your first request

1. Sign in to your panel. Open your account menu in the lower left, choose **My profile**, then open **… → API tokens** in the drawer header.
2. Create a named token with the **`server.view`** scope and an expiry of 1–365 days. Copy the secret while it is displayed. You can revoke it from the same menu.
3. Send this request over HTTPS to your panel, replacing `<token>` with the secret.

```http
GET /api/v1/servers HTTP/1.1
Host: campfire.example.org
Authorization: Bearer <token>
```

The `200 OK` response is an array of servers. This example shows three fields from one entry. Use its `id` when a route calls for `{id}`. An empty array means this credential has no visible servers.

```json
[
  {
    "id": "example-survival",
    "name": "Survival",
    "permissions": ["server.view"]
  }
]
```

## Choose scopes

Campfire checks both the token's scopes and its owner's current permissions on the requested resource. For example, an account with access to two servers can have different effective permissions on each. Read the `permissions` array on each server before offering an action.

| Task                                            | Scopes to select                                                                  |
| ----------------------------------------------- | --------------------------------------------------------------------------------- |
| Read inventory, status, metrics, or game events | `server.view`                                                                     |
| Follow live console output                      | `server.view` and `server.console.read`                                           |
| Start or stop a server                          | `server.view` and `server.operate`                                                |
| Download an existing server backup              | `server.view` and `server.backups.download`                                       |
| Run an existing restart schedule                | `server.view`, `server.schedules.run`, and access to perform the scheduled action |
| Export platform audit history                   | `audit.view` on an account with audit access                                      |
| Read and mark the account's notifications       | `notifications.read` and `notifications.write`                                    |

Include `server.view` when selecting any other `server.*` scope. Campfire adds prerequisite scopes at creation. For example, `server.files.read` also includes the underlying operation and console-reading permissions. The [server permission hierarchy](/Campfire/authentication/#permission-hierarchy) and [platform scope catalog](/Campfire/authentication/#platform-token-scopes) list the full set. Select `*` alone for all access held by the account. The reference calls out the few operations that require it.

## HTTP conventions

- Send `Authorization: Bearer <token>` on each authenticated request over HTTPS. Standalone clients can omit cookies, `Origin`, and CSRF headers. Browser session writes use the session cookie and `X-CSRF-Token`. The [/auth and /tokens routes](/Campfire/api/administration/#browser-account-routes) are for the panel's interactive sign-in and account flows.
- Request bodies use JSON. Send one JSON object with `Content-Type: application/json`. Send `{}` when an endpoint requires an empty object. Requests without a body, such as `GET /servers`, do not need a `Content-Type` header.
- Responses default to JSON. File downloads use `application/octet-stream`, backup downloads use `application/gzip`, game-event attachments use their recorded media type, and the live console uses `text/event-stream`. Browser OIDC sign-in uses HTTP redirects.
- `201` means a resource was created. `202` means work was accepted and may still fail. Follow its status or execution history. `204` has no response body.
- Percent-encode path segments and query values that contain spaces or other reserved characters.
- A **page** in the reference accepts `page` (default `1`) and `page_size` (default `20`, maximum `100`). Plain arrays such as `GET /servers` have no page envelope. An empty page looks like this:

```json
{
  "items": [],
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total_items": 0,
    "total_pages": 0
  }
}
```

### Errors and retries

API errors generally include a JSON `error` string for display. Branch on the HTTP status. For example, a reverse proxy can return a non-JSON error before the request reaches Campfire.

| HTTP status         | What the client should do                                                                                                     |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `400`, `413`, `415` | Check the request fields, size limit, and content type respectively.                                                          |
| `401`               | Supply a valid credential. Check expiry or revocation before trying again.                                                    |
| `403`, `404`        | Check the token's scopes, current grants, and resource ID. A server outside the account's visibility can return `404`.        |
| `409`               | Read current state before acting again. Another job, a stale file revision, or a conflicting allocation can prevent a change. |
| `429`               | Honor `Retry-After` when present.                                                                                             |
| `5xx`               | Retry reads with bounded backoff. If a write timed out, inspect the resource, job, or execution before repeating it.          |

## Find an endpoint

| To work with                                                               | Read                                                         |
| -------------------------------------------------------------------------- | ------------------------------------------------------------ |
| Server status, lifecycle requests, console streaming, and audit pagination | [Integration guide](/Campfire/api/guide/)                    |
| Servers, lifecycle actions, metrics, events, and console streams           | [Servers and monitoring](/Campfire/api/servers/)             |
| File operations, backup archives, and imports                              | [Files and backups](/Campfire/api/storage/)                  |
| Schedules, execution history, copies, transfers, jobs, and audit           | [Schedules, jobs, and audit](/Campfire/api/automation/)      |
| Notifications, public status, and Discord                                  | [Notifications and publishing](/Campfire/api/publishing/)    |
| Provisioning, Sparks, Embers, accounts, roles, and grants                  | [Administration and accounts](/Campfire/api/administration/) |
