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
configuration, installation, and deletion. The role editor shows these grants in
a dependency tree and offers presets. Selecting a grant also selects its
prerequisites; removing a prerequisite immediately removes its dependent grants
without an additional confirmation. `server.view` is always selected for a
server role. Parent permissions do not automatically select their children.

### Permission hierarchy

Each child requires its parent and all of that parent's ancestors. The role
editor uses this same tree:

```text
server.view
└── server.console.read
    └── server.operate
        ├── server.console.write
        ├── server.configure
        ├── server.reinstall
        ├── server.files.read
        │   └── server.files.write
        │       └── server.files.delete
        ├── server.backups.read
        │   ├── server.backups.create
        │   ├── server.backups.restore
        │   ├── server.backups.download
        │   └── server.backups.delete
        ├── server.schedules.read
        │   ├── server.schedules.manage
        │   └── server.schedules.run
        └── server.delete
```

| Grant                     | Access and prerequisites                                                                                         |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `server.view`             | Server overview and status                                                                                       |
| `server.console.read`     | Read logs; requires server viewing                                                                               |
| `server.operate`          | Initial installation, start, stop, restart, and kill; includes console reading                                   |
| `server.console.write`    | Send console commands; requires operation                                                                        |
| `server.files.read`       | Browse, read, and download files, including SFTP; requires operation                                             |
| `server.files.write`      | Upload, edit, rename, archive, extract, and chmod; includes file reading                                         |
| `server.files.delete`     | Delete files; includes file writing                                                                              |
| `server.backups.read`     | List backups; requires operation                                                                                 |
| `server.backups.create`   | Create backups; requires backup listing                                                                          |
| `server.backups.restore`  | Restore backups; requires backup listing and inherits operation                                                  |
| `server.backups.download` | Download backup archives; requires backup listing                                                                |
| `server.backups.delete`   | Delete backups; requires backup listing                                                                          |
| `server.configure`        | Server settings and backup policy; requires operation                                                            |
| `server.reinstall`        | Game update and reinstallation, including clean installation; requires operation                                 |
| `server.delete`           | Delete the server; requires operation                                                                            |
| `server.schedules.read`   | View schedules and execution history; requires operation                                                         |
| `server.schedules.manage` | Create, edit, enable, and delete schedules; requires schedule reading and the scheduled action's grant when used |
| `server.schedules.run`    | Run schedules and retry executions; requires schedule reading and the scheduled action's grant when used         |

Every server grant except `server.view` has one parent and ultimately requires
`server.view`.
Configuration, reinstallation, files, backups, and schedules are branches under
`server.operate`, which inherits console reading. Reinstallation does not require
configuration; restoration is under backup viewing. Their grants authorize those
operations without file browser grants. Clean reinstallation uses the same
`server.reinstall` grant and does not require file deletion. Selecting a child
selects its complete chain of ancestors;
selecting a parent does not grant its children or other branches.

File deletion, backup deletion, and server deletion remain separate grants, shown
in red in the role editor. Server deletion requires operation, but operation
alone does not allow deletion. File permission changes such as chmod are part of
file writing. Backup storage and retention settings are part of configuration.

### Permission presets

Applying a preset replaces the current checkbox selection. Each preset selects
the following grants and their ancestors; you can then adjust individual boxes.

| Preset               | Selected grants                                                                                                                               |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| Log viewer           | `server.console.read`: logs and live console output                                                                                           |
| Basic operator       | `server.operate`: initial installation and basic power controls, with console reading                                                         |
| Backup maintainer    | `server.backups.create`, `server.backups.restore`, and `server.backups.download`, with backup viewing and operation; excludes backup deletion |
| Server administrator | Every server grant except `server.delete`; includes file and backup deletion                                                                  |

Server administrator applies only to assigned servers. It does not grant
platform administration, ownership, or permission to change server access grants.
The built-in platform **Admin** role retains exclusive Spark, user, and role
administration. There is no separate sharing grant.

Basic operator grants only overview access, console reading, and basic operation.
The built-in **Operator** role has broader grants: console commands, file
management including deletion, backup viewing, creation, restoration and
downloading, and schedule viewing.

### Existing roles on upgrade

On upgrade, existing operator roles and API token scopes receive explicit feature
grants. This migration runs once. Overview-only grants remain overview-only;
viewing schedules requires operation access under the new hierarchy. The built-in
Operator role retains its existing access.

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
