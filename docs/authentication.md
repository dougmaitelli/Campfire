---
title: Accounts and access
---

Complete [first-run setup](/Campfire/getting-started/) to create
the initial administrator. Administrators create additional password accounts
from **Users**; there is no public password registration.

## Roles and server membership

Built-in platform roles are **Admin**, **Server creator**, and **User**.
Administrators can define custom platform and server roles on **Roles**.

Server creators need creation access to the chosen Spark. Administrators grant
that access in the Spark's settings. Server ownership and server memberships
control access to individual servers; assign members the permissions they need
for viewing, operating, or configuring that server. Only its owner and panel
administrators can change its access grants.

Server roles separate power controls, console access, files, backups, schedules,
configuration, installation, and deletion. The role editor groups these grants
and offers presets. Selecting a grant also selects its prerequisites; removing
a prerequisite removes dependent grants after confirmation.

| Grant                                           | Access and prerequisites                                                                                                        |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `server.view`                                   | Server overview and status                                                                                                      |
| `server.console.read`                           | Read logs; requires server viewing                                                                                              |
| `server.operate`                                | Power controls; includes console reading                                                                                        |
| `server.console.write`                          | Send console commands; requires operation                                                                                       |
| `server.files.read`                             | Browse, read, and download files, including SFTP                                                                                |
| `server.files.write`                            | Upload, edit, rename, archive, extract, and chmod; includes file reading                                                        |
| `server.files.delete`                           | Delete files; includes file writing                                                                                             |
| `server.backups.read`                           | List backups                                                                                                                    |
| `server.backups.create`, `.download`, `.delete` | Separate backup actions; each includes backup listing                                                                           |
| `server.backups.restore`                        | Restore backups; requires backup listing, file writing, and operation                                                           |
| `server.configure`                              | Server settings and backup policy                                                                                               |
| `server.reinstall`                              | Game update/reinstallation; requires configuration, operation, and file writing. Clean installation also requires file deletion |
| `server.delete`                                 | Delete the server                                                                                                               |
| `server.schedules.read`                         | View schedules and execution history                                                                                            |
| `server.schedules.manage`, `.run`               | Manage schedules or run them immediately; includes schedule reading and requires the underlying action's grant when used        |

Every server grant ultimately requires `server.view`. File writing, console
commands, and backup access are independent of power controls unless explicitly
listed as prerequisites. Having the prerequisites alone does not grant an action:
file writing and operation, for example, do not grant backup restoration.

On upgrade, existing roles and API token scopes receive explicit grants matching
their previous capabilities. This migration runs once; newly created roles can
use the narrower grants. The built-in Operator role retains its existing access.

## OpenID Connect

Configure the provider using the [panel environment example](/Campfire/configuration/)
and register `PUBLIC_URL/api/v1/auth/oidc/callback` as the callback URL.
Complete local administrator setup first. Before setting
`CAMPFIRE_DISABLE_LOCAL_LOGIN=true`, link an administrator to the provider and
verify that OIDC signs in to that account. First-time OIDC users receive ordinary
user access rather than administrator privileges.

```dotenv
CAMPFIRE_OIDC_ISSUER=https://identity.example.com
CAMPFIRE_OIDC_CLIENT_ID=campfire
CAMPFIRE_OIDC_CLIENT_SECRET=replace-with-provider-secret
```

Restart or recreate the panel after changing its environment. Keep local login
enabled until you have verified administrator access through your provider.

## Personal API tokens

Open **API tokens** from your account menu. Create a named token, choose its
expiry and scopes, and copy the secret when shown; it cannot be retrieved later.
Send it as `Authorization: Bearer TOKEN` when calling the API.

A token can only use its account's current permissions. Scopes narrow that
access; they do not grant additional rights. Revoke tokens from the same dialog
when they are no longer needed. Both local and OIDC users can create tokens.
Server scopes use the same prerequisites as roles. Effective access is the
intersection of the account's current grants and the token's scopes; missing
prerequisites remove dependent grants from that intersection. SFTP checks file
grants on each operation. Scheduled executions retain the scopes of the token
that authorized them and recheck current account access before dispatch.
Changing server access grants requires an owner or administrator session, or
their full-access token.

For a forgotten administrator password or an unavailable identity provider,
see [password recovery](/Campfire/operations/#administrator-password-recovery).
