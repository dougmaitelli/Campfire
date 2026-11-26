import { type PermissionID, isPermissionID } from "./permissions";
import { RoleScope } from "../contracts/domains";
import { isRecord as object, isEnumValue } from "../utils/validation";
import i18n from "../i18n";
import { apiFetch, readJSON, API_READ_TIMEOUT_MS, jsonRequest } from "./client";
import { isSpark, type Spark } from "./sparks";
import { isEmber, type Ember } from "./embers";
import { isServerConfiguration, type ServerConfiguration } from "./servers";

export interface Role {
  id: number;
  name: string;
  scope: RoleScope;
  builtin: boolean;
  permissions: PermissionID[];
}

export interface User {
  oidc_name_managed?: boolean;
  oidc_email_managed?: boolean;
  deleted_at?: string | null;
  id: number;
  username: string;
  name: string;
  email: string;
  role_id: number;
  role: { id: number; name: string; scope: RoleScope };
  disabled: boolean;
  created_at: number;
}

export interface UserInput {
  name: string;
  email: string;
  username: string;
  password: string;
  role_id: number;
  disabled: boolean;
}

export interface ServerAccess {
  grants: AccessGrant[];
  subject_roles: Role[];
  subject_users: AccessUser[];
  owner: User | null;
  members: {
    user_id: number;
    username: string;
    role_id: number;
    role: string;
  }[];
  roles: Role[];
}

export interface AccessUser {
  id: number;
  username: string;
  name: string;
}

function isAccessUser(value: unknown): value is AccessUser {
  return (
    object(value) &&
    id(value.id) &&
    typeof value.username === "string" &&
    typeof value.name === "string"
  );
}

interface SparkAccess {
  grants: AccessGrant[];
  subject_roles: Role[];
  subject_users: AccessUser[];
}

export interface AccessGrant {
  id: number;
  user_id: number | null;
  username: string;
  role_id: number | null;
  role_name: string;
  access_role_id: number;
  access_role: string;
}

function isGrant(value: unknown): value is AccessGrant {
  return (
    object(value) &&
    id(value.id) &&
    (value.user_id === null || id(value.user_id)) &&
    (value.role_id === null || id(value.role_id)) &&
    (value.user_id === null) !== (value.role_id === null) &&
    typeof value.username === "string" &&
    typeof value.role_name === "string" &&
    id(value.access_role_id) &&
    typeof value.access_role === "string"
  );
}

export async function readSparkAccess(
  sparkID: string,
  signal: AbortSignal,
): Promise<SparkAccess> {
  return readSparkAccessOptions(
    `/api/v1/sparks/${encodeURIComponent(sparkID)}/access`,
    signal,
  );
}

export function sparkAccessOptions(signal: AbortSignal): Promise<SparkAccess> {
  return readSparkAccessOptions("/api/v1/sparks/access-options", signal);
}

async function readSparkAccessOptions(
  endpoint: string,
  signal: AbortSignal,
): Promise<SparkAccess> {
  const body = await readJSON(
    await apiFetch(endpoint, {
      signal,
    }),
  );
  if (
    !object(body) ||
    !Array.isArray(body.grants) ||
    !body.grants.every(isGrant) ||
    !Array.isArray(body.subject_roles) ||
    !body.subject_roles.every(isRole) ||
    !Array.isArray(body.subject_users) ||
    !body.subject_users.every(isAccessUser)
  )
    throw new Error(i18n.t("access.unexpectedSparkAccessResponse"));
  return {
    grants: body.grants,
    subject_roles: body.subject_roles,
    subject_users: body.subject_users,
  };
}

export async function saveAccessGrants(
  endpoint: string,
  grants: AccessGrant[],
  removedIDs: number[],
): Promise<void> {
  const body = await readJSON(
    await apiFetch(endpoint, {
      signal: AbortSignal.timeout(API_READ_TIMEOUT_MS),
    }),
  );
  if (
    !object(body) ||
    !Array.isArray(body.grants) ||
    !body.grants.every(isGrant)
  )
    throw new Error(i18n.t("access.unexpectedAccessResponse"));
  const current = body.grants;
  const sameSubject = (a: AccessGrant, b: AccessGrant) =>
    a.role_id === null && b.role_id === null
      ? a.username.toLowerCase() === b.username.toLowerCase()
      : a.role_id !== null && a.role_id === b.role_id;
  for (const grant of grants) {
    if (
      current.some(
        (item) =>
          sameSubject(item, grant) &&
          item.access_role_id === grant.access_role_id,
      )
    )
      continue;
    await send(`${endpoint}/grants`, "POST", {
      ...(grant.role_id === null
        ? { username: grant.username }
        : { role_id: grant.role_id }),
      access_role_id: grant.access_role_id,
    });
  }
  for (const grantID of removedIDs) {
    const removed = current.find((item) => item.id === grantID);
    if (removed && !grants.some((item) => sameSubject(item, removed)))
      await send(`${endpoint}/grants/${grantID}`, "DELETE");
  }
}

export interface ServerOptions {
  s3_available: boolean;
  sparks: Spark[];
  embers: Ember[];
  ember_configuration?: ServerConfiguration;
  ember_managed_changes?: number;
}

function id(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}
function isRole(value: unknown): value is Role {
  return (
    object(value) &&
    id(value.id) &&
    typeof value.name === "string" &&
    isEnumValue(RoleScope, value.scope) &&
    typeof value.builtin === "boolean" &&
    Array.isArray(value.permissions) &&
    value.permissions.every(isPermissionID)
  );
}
export function isUser(value: unknown): value is User {
  return (
    object(value) &&
    id(value.id) &&
    typeof value.username === "string" &&
    typeof value.name === "string" &&
    typeof value.email === "string" &&
    (value.oidc_name_managed === undefined ||
      typeof value.oidc_name_managed === "boolean") &&
    (value.oidc_email_managed === undefined ||
      typeof value.oidc_email_managed === "boolean") &&
    id(value.role_id) &&
    object(value.role) &&
    id(value.role.id) &&
    typeof value.role.name === "string" &&
    isEnumValue(RoleScope, value.role.scope) &&
    typeof value.disabled === "boolean" &&
    typeof value.created_at === "number" &&
    Number.isSafeInteger(value.created_at)
  );
}
function isAccess(value: unknown): value is ServerAccess {
  return (
    object(value) &&
    Array.isArray(value.grants) &&
    value.grants.every(isGrant) &&
    Array.isArray(value.subject_roles) &&
    value.subject_roles.every(isRole) &&
    Array.isArray(value.subject_users) &&
    value.subject_users.every(isAccessUser) &&
    (value.owner === null || isUser(value.owner)) &&
    Array.isArray(value.roles) &&
    value.roles.every(isRole) &&
    Array.isArray(value.members) &&
    value.members.every(
      (member: unknown) =>
        object(member) &&
        id(member.user_id) &&
        typeof member.username === "string" &&
        id(member.role_id) &&
        typeof member.role === "string",
    )
  );
}
async function send(
  url: string,
  method: string,
  input?: unknown,
): Promise<unknown> {
  const response = await apiFetch(url, {
    method,
    ...(input === undefined ? {} : jsonRequest(input)),
  });
  if (response.status === 204) return null;
  return readJSON(response);
}

export async function listUsers(signal: AbortSignal): Promise<User[]> {
  const body = await readJSON(await apiFetch("/api/v1/users", { signal }));
  if (!Array.isArray(body) || !body.every(isUser))
    throw new Error(i18n.t("access.unexpectedUserListResponse"));
  return body;
}
export async function saveUser(
  input: UserInput,
  original?: User | null,
): Promise<User | null> {
  const changes: Partial<Omit<UserInput, "username">> = {};
  if (original) {
    if (input.name !== original.name) changes.name = input.name;
    if (input.email !== original.email) changes.email = input.email;
    if (input.role_id !== original.role_id) changes.role_id = input.role_id;
    if (input.disabled !== original.disabled) changes.disabled = input.disabled;
    if (input.password) changes.password = input.password;
    if (Object.keys(changes).length === 0) return null;
  }
  const body = await send(
    original ? `/api/v1/users/${original.id}` : "/api/v1/users",
    original ? "PATCH" : "POST",
    original ? changes : input,
  );
  if (!isUser(body)) throw new Error(i18n.t("access.unexpectedUserResponse"));
  return body;
}
export async function deleteUser(userID: number): Promise<void> {
  await send(`/api/v1/users/${userID}`, "DELETE");
}

export async function listRoles(signal: AbortSignal): Promise<Role[]> {
  const body = await readJSON(await apiFetch("/api/v1/roles", { signal }));
  if (!Array.isArray(body) || !body.every(isRole))
    throw new Error(i18n.t("access.unexpectedRoleListResponse"));
  return body;
}
export async function saveRole(
  input: Pick<Role, "name" | "scope" | "permissions">,
  roleID?: number,
): Promise<Role> {
  const body = await send(
    roleID ? `/api/v1/roles/${roleID}` : "/api/v1/roles",
    roleID ? "PUT" : "POST",
    input,
  );
  if (!isRole(body)) throw new Error(i18n.t("access.unexpectedRoleResponse"));
  return body;
}
export async function deleteRole(roleID: number): Promise<void> {
  await send(`/api/v1/roles/${roleID}`, "DELETE");
}

export async function serverOptions(
  signal: AbortSignal,
  serverID?: string,
): Promise<ServerOptions> {
  const body = await readJSON(
    await apiFetch(
      serverID
        ? `/api/v1/servers/${encodeURIComponent(serverID)}/editor`
        : "/api/v1/provisioning",
      { signal },
    ),
  );
  if (
    !object(body) ||
    !Array.isArray(body.sparks) ||
    !body.sparks.every(isSpark) ||
    !Array.isArray(body.embers) ||
    !body.embers.every(isEmber) ||
    typeof body.s3_available !== "boolean" ||
    (serverID !== undefined &&
      !isServerConfiguration(body.ember_configuration)) ||
    (body.ember_configuration !== undefined &&
      !isServerConfiguration(body.ember_configuration)) ||
    (body.ember_managed_changes !== undefined &&
      (!Number.isSafeInteger(body.ember_managed_changes) ||
        (body.ember_managed_changes as number) < 0))
  )
    throw new Error(i18n.t("access.unexpectedServerOptionsResponse"));
  return {
    sparks: body.sparks,
    embers: body.embers,
    s3_available: body.s3_available,
    ...(body.ember_configuration !== undefined
      ? { ember_configuration: body.ember_configuration }
      : {}),
    ...(body.ember_managed_changes !== undefined
      ? { ember_managed_changes: body.ember_managed_changes as number }
      : {}),
  };
}
export async function readServerAccess(
  serverID: string,
  signal: AbortSignal,
): Promise<ServerAccess> {
  const body = await readJSON(
    await apiFetch(`/api/v1/servers/${encodeURIComponent(serverID)}/access`, {
      signal,
    }),
  );
  if (!isAccess(body))
    throw new Error(i18n.t("access.unexpectedServerAccessResponse"));
  return body;
}
export async function serverAccessOptions(
  signal: AbortSignal,
): Promise<ServerAccess> {
  const body = await readJSON(
    await apiFetch("/api/v1/provisioning/access", { signal }),
  );
  if (!isAccess(body))
    throw new Error(i18n.t("access.unexpectedServerAccessOptionsResponse"));
  return body;
}

export async function transferOwner(
  serverID: string,
  userID: number,
): Promise<void> {
  await send(`/api/v1/servers/${encodeURIComponent(serverID)}/owner`, "PUT", {
    user_id: userID,
  });
}
