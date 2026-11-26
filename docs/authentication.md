---
title: Accounts and access
---

Complete [first-run setup](/Campfire/getting-started/) to create
the initial administrator. Administrators create additional password accounts
from **Users**; there is no public password registration.

## Accounts and password recovery

Account creation requires an email address. An administrator can reset a local
user's forgotten password from **Users**. A locked-out administrator can use the
[host recovery command](/Campfire/operations/#administrator-password-recovery).
For public registration and self-service recovery, configure an OIDC provider
that offers those features and enable them there.

## Install Campfire as an app

Open your panel over HTTPS and use your browser's install prompt or menu to
add Campfire to your home screen, dock, or app launcher. Installation options
vary by browser:

- **Chrome / Edge:** use the browser's install prompt, address-bar icon, or app menu.
- **iPhone / iPad:** in Safari, choose **Share → Add to Home Screen**. If shown,
  leave **Open as Web App** enabled.
- **Safari on supported macOS versions:** choose **File → Add to Dock**.

Campfire still requires a connection to the panel. Existing session limits
apply, and some platforms may require signing in again inside the installed app.

## Roles and server membership

Built-in platform roles are **Admin**, **Server creator**, and **User**.
Administrators can define custom platform roles and server access roles on **Roles**.
Custom platform roles can grant `servers.create`; they cannot grant platform
administration. Server access roles grant permissions on assigned servers only.

### Creating servers and granting access

Administrators grant Spark provisioning access to individual users or platform
roles in the Spark's settings. This lets those users create servers on that Spark,
including users with the **User** platform role; they do not need to be assigned
**Server creator** first. Conversely, the **Server creator** role does not allow
creation on every Spark: non-administrators still need a grant for the chosen
Spark. Provisioning access does not grant Spark administration.

Server ownership and server memberships control access to individual servers.
Owners receive all server grants through the built-in **Owner** role, including
server deletion. Members receive the grants of their assigned server access role,
such as the built-in **Operator** role or a custom role. Only the actual owner and
panel administrators can change server access grants; a role or preset with broad
server permissions does not confer ownership.

### Give someone access to a server

1. Sign in as the server's owner or a panel administrator. The person receiving
   access must already have a Campfire account.
2. Open the server and choose **More server actions → Edit server**, then open
   the **Access** step.
3. Under **Access grants**, select **User**, choose the account, and select its
   **Server access role**. Choose **Add access**.
4. Continue to the review step and choose **Save changes**.

To share with everyone assigned to a platform role, select **Platform role**
instead of **User**. This grants the selected server access role on this server;
it does not change anyone's platform role. Remove a grant from the same Access
step and save to revoke it. Other grants or ownership can still provide access.

### Server access roles

Server access roles separate power controls, console access, files, backups, schedules,
configuration, installation, and deletion. The role editor shows these grants in
a dependency tree and offers presets. Selecting a grant also selects its
prerequisites; removing a prerequisite immediately removes its dependent grants
without an additional confirmation. `server.view` is always selected for a
server access role. Parent permissions do not automatically select their children.

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

| Grant                     | Access and prerequisites                                                                                          |
| ------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `server.view`             | Overview, status, resource metrics/history, game events/media, and permission-filtered Activity                   |
| `server.console.read`     | Read logs; requires server viewing                                                                                |
| `server.operate`          | Initial installation, power controls, saved-configuration sync, and log-alert dismissal; includes console reading |
| `server.console.write`    | Send console commands; requires operation                                                                         |
| `server.files.read`       | Browse, read, and download files, including SFTP; requires operation                                              |
| `server.files.write`      | Create directories, upload, edit, move, rename, archive, extract, and chmod; includes file reading                |
| `server.files.delete`     | Delete files; includes file writing                                                                               |
| `server.backups.read`     | List backups; requires operation                                                                                  |
| `server.backups.create`   | Create, import, rename, and pin/unpin backups; requires backup listing                                            |
| `server.backups.restore`  | Restore backups; requires backup listing and inherits operation                                                   |
| `server.backups.download` | Download backup archives; requires backup listing                                                                 |
| `server.backups.delete`   | Delete backups; requires backup listing                                                                           |
| `server.configure`        | Server settings, backup policy, and per-server Discord integrations; requires operation                           |
| `server.reinstall`        | Game update and reinstallation, including clean installation; requires operation                                  |
| `server.delete`           | Delete the server; requires operation                                                                             |
| `server.schedules.read`   | View schedules and execution history; requires operation                                                          |
| `server.schedules.manage` | Create, edit, enable, and delete schedules; requires schedule reading and the scheduled action's grant when used  |
| `server.schedules.run`    | Run schedules and retry executions; requires schedule reading and the scheduled action's grant when used          |

Reinstallation and backup restoration do not require file-browser grants. Clean
reinstallation uses `server.reinstall`, not file deletion. File, backup, and
server deletion are separate grants, highlighted in red in the role editor.

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
The built-in platform **Admin** role retains exclusive platform administration,
including Sparks, users, roles, Embers, global Discord connections, server ownership
reassignment, and transfers between Sparks. There is no separate sharing grant.

Basic operator grants only overview access, console reading, and basic operation.
The built-in **Operator** role has broader grants: console commands, file
management including deletion, backup viewing, creation, restoration and
downloading, and schedule viewing.

### Permissions for related features

- **Activity:** `server.view` permits opening Activity and reading game events and
  media. Manual operation history additionally requires `server.operate`;
  scheduled operation history requires `server.schedules.read`. Job details and
  notification actions are filtered by their corresponding permissions, so
  overview access does not expose every entry or action.
- **Ember updates:** `server.configure` permits editing saved server settings, but
  applying an updated Ember definition requires the platform
  `servers.manage_all` permission. See [server configuration](/Campfire/servers/).
- **Discord:** per-server integration settings require `server.configure`.
  Setting up global Discord connections requires Admin access; an API token must
  also have full-access scope. See [Discord integration](/Campfire/discord/).
- **Copies:** creating a copy requires `servers.create`, source
  `server.configure`, and provisioning access on the destination Spark. Including
  game files also requires source `server.files.read`.
- **Transfers:** moving an existing server to another Spark requires both
  `sparks.manage` and `servers.manage_all`; ownership or server operation grants
  alone are insufficient. See [copies and transfers](/Campfire/server-transfers/)
  for job permissions and recovery.

## View as another user

The built-in platform **Admin** role can open **View as…** from the account menu
and choose another enabled user. The panel then uses that user's current account
role, server memberships, ownership, and Spark provisioning grants for its normal
permission checks. Navigation, visible servers, files, backups, schedules, and
controls reflect the selected user's access.

This is an inspect-only preview. Changes are rejected by the API, including
server operations, file writes, schedule changes, and account changes. File reads,
downloads, and live console output remain available when the selected user's
permissions allow them. Personal API tokens are unavailable during preview.
The banner identifies the selected user and provides **Return to Admin**.

Only authenticated Admin browser sessions can start a preview; server owners,
custom roles, and API tokens cannot. Preview does not create credentials or a
session for the selected user. Its cookie is bound to the Admin's current session,
persists across reloads, and applies to other tabs using that session. Open tabs
refresh when the preview starts or stops. Start, stop, and rejected changes are
audited under the real administrator's account.

Permissions are rechecked on subsequent requests, including live console
reauthorization. Disabling or deleting the selected account blocks further preview
requests; refreshing the session ends the preview and explicitly returns to the
administrator's account. Signing out ends the administrator's session.

## OpenID Connect

Configure the provider using the [panel environment example](/Campfire/configuration/)
and register `PUBLIC_URL/api/v1/auth/oidc/callback` as the callback URL.
Complete local administrator setup first. Before setting
`CAMPFIRE_DISABLE_LOCAL_LOGIN=true`, link an administrator to the provider and
verify that OIDC signs in to that account. Newly provisioned OIDC users receive
ordinary user access rather than administrator privileges.

Creating an account on first OIDC sign-in requires the provider to supply a
valid email address. Later sign-ins update the name and email supplied by the
provider and preserve previous values when a claim is absent. New accounts use
the provider's preferred username, normalized to Campfire's username format. A unique suffix is added only when that username is already
in use. Existing OIDC accounts keep their usernames on subsequent sign-ins.

```dotenv
CAMPFIRE_OIDC_ISSUER=https://identity.example.com
CAMPFIRE_OIDC_CLIENT_ID=campfire
CAMPFIRE_OIDC_CLIENT_SECRET=replace-with-provider-secret
```

Restart or recreate the panel after changing its environment. Keep local login
enabled until you have verified administrator access through your provider.

### Automatic account linking

To grant access before a user signs in, enable `CAMPFIRE_OIDC_AUTO_LINK_EMAIL=true`
(or `--oidc-auto-link-email`). In **Users**, create the account with the email
your provider will verify. Leave the password blank for OIDC-only sign-in, then
assign the platform role and server grants. On first sign-in, Campfire links the
provider's subject to that account when the provider reports `email_verified: true`
for the assigned address. The user keeps the same ID and all assigned permissions.
If an account linked to the configured provider already lists the address,
Campfire reports the conflict in **Users**. Confirm the intended user's identity
with your provider before granting access to an existing account. If the
provider has the wrong address, correct it there and have the account holder
sign in again to refresh their Campfire profile.
A disabled account, an unverified claim for a reserved address, or an attempt
to claim an already-linked account is rejected.

Each admin-assigned address can be reserved for one account. Addresses entered
during setup, admin creation, or an admin email change are eligible. Editing your own email withdraws that eligibility. Matching preserves
the exact local-part and normalizes the domain. Older profiles of unknown origin
can use explicit linking from an authenticated session. With automatic linking
off, or without a matching assignment, a new identity gets an ordinary user
account. Later sign-ins use issuer and subject, even if the provider changes
the user's email. Each user can link one subject per provider.

## Personal API tokens

Open **My profile** from your account menu, then choose **API tokens** from the
header’s **…** actions menu. Create a named token, choose its
expiry and scopes, and copy the secret when shown; it cannot be retrieved later.
Send it as `Authorization: Bearer TOKEN` when calling the API.

For request examples, response formats, and the endpoint reference, see the
[panel API](/Campfire/api/).

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

### Platform token scopes

Platform token scopes are separate from the grants available to custom platform
roles. Selecting an administrative scope does not make an ordinary account an
administrator. Admin tokens can be narrowed to specific platform actions; custom
platform roles can grant only `servers.create`.

| Scope                  | Limits token access to                                                                                                      |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `*`                    | All access the account already has; required for ownership-based access management and global Discord connection management |
| `servers.create`       | Server creation and provisioning options, subject to Spark access                                                           |
| `servers.manage_all`   | Platform server management, including ownership reassignment and applying Ember definitions                                 |
| `sparks.manage`        | Spark administration; transfers also require `servers.manage_all`                                                           |
| `embers.manage`        | Ember definition management                                                                                                 |
| `users.manage`         | User administration                                                                                                         |
| `roles.manage`         | Role administration                                                                                                         |
| `audit.view`           | Platform audit history                                                                                                      |
| `notifications.read`   | Reading the account's notifications                                                                                         |
| `notifications.write`  | Marking the account's notifications as read                                                                                 |
| `notifications.manage` | Notification delivery status, testing, and retries                                                                          |

Server-specific endpoints still require their corresponding server scopes.
Platform scopes do not replace the server permission hierarchy or bypass token
scope checks.

For a forgotten administrator password or an unavailable identity provider,
see [password recovery](/Campfire/operations/#administrator-password-recovery).

## Active sessions

Open **My profile** from the account menu, then choose **Active sessions** from
the header’s **…** actions menu to see your signed-in browsers,
sign-in times, and recent activity. **This session** identifies the current browser.
Sign out any individual session, or choose **Sign out all other devices** to keep
only the current browser signed in. Signing out the current session returns you to
the login screen. Session management is unavailable during **View as**.

Browser and device labels come from the browser’s user agent. Sessions created
before this feature may show unavailable device details or sign-in times. Last
activity is updated periodically, rather than on every request.

## My profile and two-factor authentication

Open **My profile** from the account menu at the bottom left. Edit your display
name and email there for a local account, or at your identity provider for an
OIDC-linked account. Local-password users can also change their password by confirming
their current password. When 2FA is enabled, saving a password change opens a
dialog for a fresh authenticator or recovery code. New passwords must meet the
central password policy: by default, at least 15 characters including uppercase,
lowercase, a number, and a symbol. Existing passwords remain valid for sign-in.
Profile editing does not allow changing your username, role, or enabled state.

Under **Two-factor authentication**, the device table lists your authenticators.
Choose **Add authenticator**, name the device, and scan the QR code in the dialog.
A manual setup key is also available. Your signed-in browser authorizes enrollment;
you do not need to enter your password again. Confirm a six-digit code within ten
minutes to activate the device. The first device also issues ten recovery codes,
shown once. Additional devices have independent secrets and retain those recovery
codes. QR codes are generated by the panel; no external service receives your secret.

Local sign-in asks for a second factor after checking your password. Each recovery
code works once with your password. Authenticator codes cannot be reused: wait
for a fresh code if you just used one to sign in or change a security setting.
You can enroll up to ten devices and remove them independently. Removal asks for a
fresh code from any enrolled authenticator or an unused recovery code. Removing the
last device turns off 2FA and clears the recovery codes. Adding or removing a device
signs out other browsers but preserves API tokens.

If you lose both your authenticator and recovery codes, ask an administrator to
verify your identity and recover the account. If you still have a recovery code,
choose **Lost your authenticator?** at the authenticator-code step after entering
your username and password. An unused recovery code signs you in without removing
any devices or disabling MFA. Only that recovery code is consumed. Manage lost
devices from **My profile** after signing in.

An administrator can
verify your identity and open your account under **Users → Reset user’s 2FA**.
The authenticated administrator confirms the reset without re-entering a password
or MFA code. Administrators cannot use this action to reset themselves. API tokens and **View as** cannot
perform the reset. The action is audited.

A reset removes 2FA and recovery codes and signs out the affected browser
sessions. API tokens remain valid. You can sign in with your password and choose
whether to enroll again. For administrator lockouts, use the
[server-side recovery command](/Campfire/operations/#two-factor-authentication-recovery).
Password resets preserve 2FA; resetting one does not reset the other.

OIDC sign-ins continue to use the identity provider’s MFA policy. Campfire’s local
2FA settings and reset actions do not change MFA at the identity provider.
