# Accounts and access

Complete [first-run setup](../README.md#quick-start-with-docker-compose) to create
the initial administrator. Administrators create additional password accounts
from **Users**; there is no public password registration.

## Roles and server membership

Built-in platform roles are **Admin**, **Server creator**, and **User**.
Administrators can define custom platform and server roles on **Roles**.

Server creators need creation access to the chosen Spark. Administrators grant
that access in the Spark's settings. Server ownership and server memberships
control access to individual servers; assign members the permissions they need
for viewing, operating, or configuring that server.

## OpenID Connect

Configure the provider using the [panel environment example](../config/campfire.env.example)
and register `PUBLIC_URL/api/v1/auth/oidc/callback` as the callback URL.
Complete local administrator setup first. Before setting
`CAMPFIRE_DISABLE_LOCAL_LOGIN=true`, link an administrator to the provider and
verify that OIDC signs in to that account. First-time OIDC users receive ordinary
user access rather than administrator privileges.

## Personal API tokens

Open **API tokens** from your account menu. Create a named token, choose its
expiry and scopes, and copy the secret when shown; it cannot be retrieved later.
Send it as `Authorization: Bearer TOKEN` when calling the API.

A token can only use its account's current permissions. Scopes narrow that
access; they do not grant additional rights. Revoke tokens from the same dialog
when they are no longer needed. Both local and OIDC users can create tokens.

For a forgotten administrator password or an unavailable identity provider,
see [password recovery](../README.md#administrator-password-recovery).
