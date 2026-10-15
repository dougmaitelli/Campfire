import { apiFetch, readJSON } from "./client";

export interface APIToken {
  id: number;
  name: string;
  prefix: string;
  scopes: string[];
  created_at: number;
  expires_at: number;
  last_used_at: number;
}

function object(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
function strings(value: unknown): value is string[] {
  return (
    Array.isArray(value) && value.every((item) => typeof item === "string")
  );
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
    strings(value.scopes)
  );
}
export async function listTokens(signal: AbortSignal) {
  const body = await readJSON(await apiFetch("/api/v1/tokens", { signal }));
  if (
    !object(body) ||
    !Array.isArray(body.tokens) ||
    !body.tokens.every(isToken) ||
    !strings(body.scopes)
  )
    throw new Error("Unexpected API token response.");
  return { tokens: body.tokens, scopes: body.scopes };
}
export async function createToken(
  name: string,
  days: number,
  scopes: string[],
) {
  const body = await readJSON(
    await apiFetch("/api/v1/tokens", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, expires_in_days: days, scopes }),
    }),
  );
  if (!object(body) || !isToken(body.token) || typeof body.secret !== "string")
    throw new Error("Unexpected API token response.");
  return { token: body.token, secret: body.secret };
}
export async function revokeToken(id: number) {
  const response = await apiFetch(`/api/v1/tokens/${id}`, { method: "DELETE" });
  if (response.status !== 204) await readJSON(response);
}
