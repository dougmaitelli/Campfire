---
title: Administration and accounts API
description: Provision servers, administer Sparks and Embers, grant access, and identify browser-only account routes.
---

Paths are relative to `/api/v1`. Platform scopes narrow the token owner's existing rights. A tool that creates servers can use provisioning options without needing access to global Spark or Ember administration.

## Provisioning and access

| Method and path                                | Access                          | Request and result                                                                                                                                                                      |
| ---------------------------------------------- | ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /permissions`                             | Authenticated                   | Array of server permission definitions with `id`, `group`, `label`, and `requires`. For the caller's effective grants on a particular server, read that server's `permissions` instead. |
| `GET /provisioning`                            | `servers.create`                | Returns available `sparks`, `embers`, and `s3_available` for the server creation form.                                                                                                  |
| `GET /provisioning/access`                     | `servers.create`                | Returns assignable server `roles`, `subject_users` and `subject_roles` for sharing a new server.                                                                                        |
| `GET /servers/{id}/access`                     | Owner or administrator with `*` | Current `owner`, `members`, `grants`, assignable `roles`, and possible subjects.                                                                                                        |
| `POST /servers/{id}/access/grants`             | Owner or administrator with `*` | Send `access_role_id` and exactly one subject, `username` or numeric platform `role_id`. Returns `204`.                                                                                 |
| `DELETE /servers/{id}/access/grants/{grantID}` | Owner or administrator with `*` | Remove the grant by its `id`. Returns `204`.                                                                                                                                            |
| `POST /servers/{id}/members`                   | Owner or administrator with `*` | Assign one user by `username` and server `role_id`.                                                                                                                                     |
| `DELETE /servers/{id}/members/{userID}`        | Owner or administrator with `*` | Remove an individual's membership.                                                                                                                                                      |
| `PUT /servers/{id}/owner`                      | `servers.manage_all`            | Send `{"user_id":123}` to reassign ownership. Returns the updated access details.                                                                                                       |

For example, to grant a user a server role, look up the role in the server's `/access` response. If its ID is `7`, send `{"username":"alex","access_role_id":7}` to `/access/grants`. When assigning the same server role to a group, replace `username` with the group's numeric `role_id`. See [accounts and access](/Campfire/authentication/) for how the resulting grants combine.

## Sparks

All routes in this group require `sparks.manage`. A Spark represents one game host. Its configuration includes `name`, `description`, `disabled`, `game_address`, resource limits (`cpu_limit`, `memory_mb`, `disk_mb`), and `port_allocations`. A port allocation contains `bind_ip`, `start`, `end`, and `protocol`.

| Method and path                               | Request and result                                                                                                                                 |
| --------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /sparks`                                 | Array of registered Sparks and their connectivity information.                                                                                     |
| `POST /sparks`                                | Send the Spark configuration. Returns `201` with the Spark and its `Location`.                                                                     |
| `GET /sparks/{id}`                            | Read one Spark.                                                                                                                                    |
| `PATCH /sparks/{id}`                          | Send the changed configuration fields. Returns the updated Spark.                                                                                  |
| `DELETE /sparks/{id}`                         | Remove the Spark registration when eligible.                                                                                                       |
| `POST /sparks/{id}/enrollment`                | Send `{}` to issue an enrollment credential for an enabled Spark. Returns `201` with `token` and `expires_at`.                                     |
| `GET /sparks/{id}/metrics/history`            | CPU and memory history. Uses the same `from`, `to`, and `limit` parameters as [server history](/Campfire/api/servers/#inventory-and-measurements). |
| `GET /sparks/update-status`                   | Read panel-wide Spark release and update-check status.                                                                                             |
| `POST /sparks/{id}/update`                    | Send `version` and optionally `force` to reinstall that version. Returns `202` with `version`.                                                     |
| `POST /sparks/{id}/prune`                     | Send `{}` to remove eligible unused Docker resources and return the prune result.                                                                  |
| `GET /sparks/access-options`                  | Lists possible user and role subjects for Spark access.                                                                                            |
| `GET /sparks/{id}/access`                     | Returns current `grants`, `subject_users`, and `subject_roles`.                                                                                    |
| `POST /sparks/{id}/access/grants`             | Grant provisioning access to one `username` or platform `role_id`. Returns `204`.                                                                  |
| `DELETE /sparks/{id}/access/grants/{grantID}` | Remove a provisioning grant.                                                                                                                       |
| `GET /sparks/{id}/roles`                      | Read the `role_ids` assigned to this Spark.                                                                                                        |
| `PUT /sparks/{id}/roles`                      | Replace the assigned role list with `{"role_ids":[…]}`.                                                                                            |

## Embers

All routes in this group require `embers.manage`. Preview and import accept a definition object or a wrapper containing a `definition` string, optional `source_url`, and `acknowledge_warnings`. See the [Ember schema](/Campfire/ember-schema/) for the game definition itself.

| Method and path                     | Request and result                                                                                              |
| ----------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `GET /embers`                       | List imported Ember definitions.                                                                                |
| `GET /embers/{id}`                  | Fetch a single Ember.                                                                                           |
| `POST /embers/preview`              | Preview the submitted definition and inspect import warnings.                                                   |
| `POST /embers/preview-url`          | Send `{"url":"…"}` to fetch and preview an upstream definition.                                                 |
| `POST /embers`                      | Import a definition. Returns `201`, the Ember, and its `Location`.                                              |
| `PUT /embers/{id}`                  | Replace the named Ember with the submitted definition. Returns `201` and the Ember.                             |
| `DELETE /embers/{id}`               | Remove the imported definition when eligible.                                                                   |
| `GET /embers/updates`               | List stored upstream update-check results.                                                                      |
| `POST /embers/{id}/check-update`    | Send `{}` to check the source and return update information.                                                    |
| `POST /embers/{id}/download-update` | Send `{}` to download a candidate and return its preview for review. Apply it through the replacement endpoint. |

## Users and roles

User IDs and role IDs are numbers. For an administrative user edit, prefer `PATCH` and supply only the fields the operator changed. Accounts linked to OIDC take their name and email from the provider.

| Method and path                  | Access                            | Request and result                                                                                                                                                                         |
| -------------------------------- | --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `GET /users`                     | `users.manage`                    | List users.                                                                                                                                                                                |
| `POST /users`                    | `users.manage`                    | Send `username`, `email`, `name`, `role_id`, and `password`. Optionally set `disabled`. A password can be omitted when OIDC auto-linking by email is enabled. Returns `201` with the user. |
| `PATCH /users/{userID}`          | `users.manage`                    | Update the supplied `name`, `email`, `password`, `role_id`, or `disabled`. Returns the updated user.                                                                                       |
| `PUT /users/{userID}`            | `users.manage`                    | Requires `role_id` and defaults an omitted `disabled` to false. Returns the updated user.                                                                                                  |
| `DELETE /users/{userID}`         | `users.manage`                    | Delete the user when eligible.                                                                                                                                                             |
| `POST /users/{userID}/reset-2fa` | Interactive administrator session | Remove another user's authenticators and recovery codes. Bearer tokens cannot authorize this operation.                                                                                    |
| `GET /roles`                     | `roles.manage`                    | List roles with IDs, scope, and permissions.                                                                                                                                               |
| `POST /roles`                    | `roles.manage`                    | Send `name`, `scope` (`platform` or `server`), and `permissions`. Returns `201` with the role.                                                                                             |
| `PUT /roles/{roleID}`            | `roles.manage`                    | Send the edited role definition. Returns the updated role.                                                                                                                                 |
| `DELETE /roles/{roleID}`         | `roles.manage`                    | Remove an eligible custom role.                                                                                                                                                            |

## Browser account routes

These routes serve the panel's interactive flows. Bearer credentials are rejected on `/auth/*` and `/tokens`, including token listing. For an external client, [create a personal token in the profile drawer](/Campfire/api/#make-your-first-request). Setup, login, session discovery, and the OIDC redirect/callback can be reached before a session exists. Authenticated browser writes require the session's `X-CSRF-Token`.

| Method and path                       | Purpose                                                                                                      |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `GET /auth/session`                   | Discover sign-in/setup state or read the current session and CSRF token.                                     |
| `POST /auth/setup`                    | Complete first-administrator setup with the host-issued setup token.                                         |
| `POST /auth/login`                    | Sign in with local credentials and an MFA code when required.                                                |
| `POST /auth/logout`                   | End the current sign-in.                                                                                     |
| `GET /auth/oidc/start`                | Begin an OpenID Connect redirect.                                                                            |
| `GET /auth/oidc/callback`             | Complete the provider's redirect.                                                                            |
| `POST /auth/oidc/link`                | Begin linking the signed-in account. Returns a provider URL.                                                 |
| `GET /auth/profile`                   | Read the current user's profile and local-password status.                                                   |
| `PUT /auth/profile`                   | Edit profile fields or change a local password with current credentials.                                     |
| `GET /auth/mfa`                       | Read enrollment and device status.                                                                           |
| `POST /auth/mfa/enroll`               | Begin an authenticator enrollment with a device `name`. Returns setup information and a QR code.             |
| `POST /auth/mfa/confirm`              | Confirm the pending enrollment using `code`.                                                                 |
| `DELETE /auth/mfa/devices/{deviceID}` | Remove an enrolled device with a verification `code`.                                                        |
| `DELETE /auth/mfa`                    | Disable the user's MFA with `password` and `code`.                                                           |
| `GET /auth/sessions`                  | List the user's active sessions.                                                                             |
| `DELETE /auth/sessions/{id}`          | Revoke one session.                                                                                          |
| `DELETE /auth/sessions`               | Revoke other sessions.                                                                                       |
| `POST /auth/view-as`                  | Start an administrator preview of a `user_id`.                                                               |
| `DELETE /auth/view-as`                | End that preview.                                                                                            |
| `GET /tokens`                         | Read the caller's token metadata, scopes, and server permission catalog.                                     |
| `POST /tokens`                        | Send `name`, `expires_in_days`, and `scopes`. Returns `201` with `token` metadata and the one-time `secret`. |
| `DELETE /tokens/{tokenID}`            | Revoke a personal API token. Returns `204`.                                                                  |
