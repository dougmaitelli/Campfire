import { apiFetch, readJSON } from "./client";

export interface AuthState {
  authenticated: boolean;
  setup_required: boolean;
  local_login?: boolean;
  oidc_enabled?: boolean;
  username?: string;
  name?: string;
  picture?: string;
  user_id?: number;
  role_id?: number;
  role?: string;
  admin?: boolean;
  permissions?: string[];
  csrf_token?: string;
  expires_at?: number;
}

export interface Credentials {
  username: string;
  password: string;
  setup_token?: string;
}

function isAuthState(value: unknown): value is AuthState {
  return (
    typeof value === "object" &&
    value !== null &&
    "authenticated" in value &&
    typeof value.authenticated === "boolean" &&
    "setup_required" in value &&
    typeof value.setup_required === "boolean" &&
    "local_login" in value &&
    typeof value.local_login === "boolean" &&
    "oidc_enabled" in value &&
    typeof value.oidc_enabled === "boolean" &&
    (!("name" in value) || typeof value.name === "string") &&
    (!("picture" in value) || typeof value.picture === "string") &&
    (!value.authenticated ||
      ("username" in value &&
        typeof value.username === "string" &&
        "user_id" in value &&
        typeof value.user_id === "number" &&
        Number.isSafeInteger(value.user_id) &&
        value.user_id > 0 &&
        "role_id" in value &&
        typeof value.role_id === "number" &&
        Number.isSafeInteger(value.role_id) &&
        "role" in value &&
        typeof value.role === "string" &&
        "admin" in value &&
        typeof value.admin === "boolean" &&
        "permissions" in value &&
        Array.isArray(value.permissions) &&
        value.permissions.every(
          (entry: unknown) => typeof entry === "string",
        ) &&
        "csrf_token" in value &&
        typeof value.csrf_token === "string" &&
        "expires_at" in value &&
        typeof value.expires_at === "number" &&
        Number.isSafeInteger(value.expires_at)))
  );
}

export async function readSession(signal: AbortSignal): Promise<AuthState> {
  const body = await readJSON(
    await fetch("/api/v1/auth/session", {
      credentials: "same-origin",
      cache: "no-store",
      signal,
    }),
  );
  if (!isAuthState(body))
    throw new Error("Unexpected authentication response.");
  return body;
}

export async function authenticate(
  credentials: Credentials,
  setup: boolean,
): Promise<AuthState> {
  const body = await readJSON(
    await fetch(`/api/v1/auth/${setup ? "setup" : "login"}`, {
      method: "POST",
      credentials: "same-origin",
      cache: "no-store",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(credentials),
      signal: AbortSignal.timeout(35000),
    }),
  );
  if (!isAuthState(body) || !body.authenticated)
    throw new Error("Unexpected authentication response.");
  return body;
}

export async function logout(): Promise<void> {
  const response = await apiFetch("/api/v1/auth/logout", { method: "POST" });
  if (response.status !== 204 && response.status !== 401)
    await readJSON(response);
}

export async function linkOIDC(): Promise<void> {
  const body = await readJSON(
    await apiFetch("/api/v1/auth/oidc/link", { method: "POST" }),
  );
  if (
    typeof body !== "object" ||
    body === null ||
    !("url" in body) ||
    typeof body.url !== "string"
  )
    throw new Error("Unexpected OpenID Connect response.");
  window.location.assign(body.url);
}
