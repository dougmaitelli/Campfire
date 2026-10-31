import { isRecord as object } from "../utils/validation";
import i18n from "../i18n";
import { apiFetch, readJSON, apiJSON } from "./client";
import {
  isPermissionCatalog,
  type PermissionID,
  isPermissionID,
} from "./permissions";

export interface APIToken {
  id: number;
  name: string;
  prefix: string;
  scopes: PermissionID[];
  created_at: number;
  expires_at: number;
  last_used_at: number;
}

function permissionIDs(value: unknown): value is PermissionID[] {
  return Array.isArray(value) && value.every(isPermissionID);
}
function isToken(value: unknown): value is APIToken {
  return (
    object(value) &&
    ["id", "created_at", "expires_at", "last_used_at"].every(
      (key) =>
        typeof value[key] === "number" && Number.isSafeInteger(value[key]),
    ) &&
    typeof value.name === "string" &&
    typeof value.prefix === "string" &&
    permissionIDs(value.scopes)
  );
}
export async function listTokens(signal: AbortSignal) {
  const body = await readJSON(await apiFetch("/api/v1/tokens", { signal }));
  if (
    !object(body) ||
    !Array.isArray(body.tokens) ||
    !body.tokens.every(isToken) ||
    !permissionIDs(body.scopes)
  )
    throw new Error(i18n.t("tokens.unexpectedAPITokenResponse"));
  if (!isPermissionCatalog(body.permissions))
    throw new Error(i18n.t("tokens.unexpectedAPITokenResponse"));
  return {
    tokens: body.tokens,
    scopes: body.scopes,
    permissions: body.permissions,
  };
}
export async function createToken(
  name: string,
  days: number,
  scopes: PermissionID[],
) {
  const body = await readJSON(
    await apiJSON(
      "/api/v1/tokens",
      { name, expires_in_days: days, scopes },
      { method: "POST" },
    ),
  );
  if (!object(body) || !isToken(body.token) || typeof body.secret !== "string")
    throw new Error(i18n.t("tokens.unexpectedAPITokenResponse"));
  return { token: body.token, secret: body.secret };
}
export async function revokeToken(id: number) {
  const response = await apiFetch(`/api/v1/tokens/${id}`, { method: "DELETE" });
  if (response.status !== 204) await readJSON(response);
}
