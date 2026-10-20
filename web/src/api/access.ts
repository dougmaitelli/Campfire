import { apiFetch, readJSON } from "./client";
import { isSpark, type Spark } from "./sparks";
import { isEmber, type Ember } from "./embers";

export interface Role {
  id: number;
  name: string;
  scope: "platform" | "server";
  builtin: boolean;
  permissions: string[];
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
  role: { id: number; name: string; scope: "platform" | "server" };
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
    throw new Error("Unexpected Spark access response.");
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
    await apiFetch(endpoint, { signal: AbortSignal.timeout(15000) }),
  );
  if (
    !object(body) ||
    !Array.isArray(body.grants) ||
    !body.grants.every(isGrant)
  )
    throw new Error("Unexpected access response.");
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
}

function object(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function id(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}
function isRole(value: unknown): value is Role {
  return (
    object(value) &&
    id(value.id) &&
    typeof value.name === "string" &&
    (value.scope === "platform" || value.scope === "server") &&
    typeof value.builtin === "boolean" &&
    Array.isArray(value.permissions) &&
    value.permissions.every((entry: unknown) => typeof entry === "string")
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
    (value.role.scope === "platform" || value.role.scope === "server") &&
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
    ...(input === undefined
      ? {}
      : {
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(input),
        }),
  });
  if (response.status === 204) return null;
  return readJSON(response);
}

export async function listUsers(signal: AbortSignal): Promise<User[]> {
  const body = await readJSON(await apiFetch("/api/v1/users", { signal }));
  if (!Array.isArray(body) || !body.every(isUser))
    throw new Error("Unexpected user list response.");
  return body;
}
export async function saveUser(
  input: UserInput,
  userID?: number,
): Promise<User> {
  const body = await send(
    userID ? `/api/v1/users/${userID}` : "/api/v1/users",
    userID ? "PUT" : "POST",
    userID
      ? {
          name: input.name,
          email: input.email,
          password: input.password,
          role_id: input.role_id,
          disabled: input.disabled,
        }
      : input,
  );
  if (!isUser(body)) throw new Error("Unexpected user response.");
  return body;
}
export async function deleteUser(userID: number): Promise<void> {
  await send(`/api/v1/users/${userID}`, "DELETE");
}

export async function listRoles(signal: AbortSignal): Promise<Role[]> {
  const body = await readJSON(await apiFetch("/api/v1/roles", { signal }));
  if (!Array.isArray(body) || !body.every(isRole))
    throw new Error("Unexpected role list response.");
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
  if (!isRole(body)) throw new Error("Unexpected role response.");
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
    typeof body.s3_available !== "boolean"
  )
    throw new Error("Unexpected server options response.");
  return {
    sparks: body.sparks,
    embers: body.embers,
    s3_available: body.s3_available,
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
  if (!isAccess(body)) throw new Error("Unexpected server access response.");
  return body;
}
export async function serverAccessOptions(
  signal: AbortSignal,
): Promise<ServerAccess> {
  const body = await readJSON(
    await apiFetch("/api/v1/provisioning/access", { signal }),
  );
  if (!isAccess(body))
    throw new Error("Unexpected server access options response.");
  return body;
}

export interface ServerAccessDraft {
  ownerID: number;
  members: { username: string; role_id: number }[];
  removedUserIDs: number[];
}

export async function saveServerAccessDraft(
  serverID: string,
  draft: ServerAccessDraft,
): Promise<void> {
  const current = await readServerAccess(serverID, AbortSignal.timeout(15000));
  for (const member of draft.members) {
    if (member.username.toLowerCase() === current.owner?.username.toLowerCase())
      continue;
    if (
      !current.members.some(
        (existing) =>
          existing.username.toLowerCase() === member.username.toLowerCase() &&
          existing.role_id === member.role_id,
      )
    ) {
      await shareServer(serverID, member.username, member.role_id);
    }
  }
  for (const userID of draft.removedUserIDs) {
    if (current.members.some((member) => member.user_id === userID))
      await removeMember(serverID, userID);
  }
  if (draft.ownerID && draft.ownerID !== current.owner?.id)
    await transferOwner(serverID, draft.ownerID);
}
export async function shareServer(
  serverID: string,
  username: string,
  roleID: number,
): Promise<void> {
  await send(
    `/api/v1/servers/${encodeURIComponent(serverID)}/members`,
    "POST",
    { username, role_id: roleID },
  );
}
export async function removeMember(
  serverID: string,
  userID: number,
): Promise<void> {
  await send(
    `/api/v1/servers/${encodeURIComponent(serverID)}/members/${userID}`,
    "DELETE",
  );
}
export async function transferOwner(
  serverID: string,
  userID: number,
): Promise<void> {
  await send(`/api/v1/servers/${encodeURIComponent(serverID)}/owner`, "PUT", {
    user_id: userID,
  });
}

export async function readSparkRoles(
  sparkID: string,
  signal: AbortSignal,
): Promise<number[]> {
  const body = await readJSON(
    await apiFetch(`/api/v1/sparks/${encodeURIComponent(sparkID)}/roles`, {
      signal,
    }),
  );
  if (
    !object(body) ||
    !Array.isArray(body.role_ids) ||
    !body.role_ids.every(id)
  )
    throw new Error("Unexpected Spark role response.");
  return body.role_ids;
}
export async function setSparkRoles(
  sparkID: string,
  roleIDs: number[],
): Promise<void> {
  await send(`/api/v1/sparks/${encodeURIComponent(sparkID)}/roles`, "PUT", {
    role_ids: roleIDs,
  });
}
