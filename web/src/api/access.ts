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
  id: number;
  username: string;
  name: string;
  role_id: number;
  role: { id: number; name: string; scope: "platform" | "server" };
  disabled: boolean;
  created_at: number;
}

export interface UserInput {
  name: string;
  username: string;
  password: string;
  role_id: number;
  disabled: boolean;
}

export interface ServerAccess {
  owner: User | null;
  members: {
    user_id: number;
    username: string;
    role_id: number;
    role: string;
  }[];
  roles: Role[];
}

export interface ServerOptions {
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
function isUser(value: unknown): value is User {
  return (
    object(value) &&
    id(value.id) &&
    typeof value.username === "string" &&
    typeof value.name === "string" &&
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
    !body.embers.every(isEmber)
  )
    throw new Error("Unexpected server options response.");
  return { sparks: body.sparks, embers: body.embers };
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
