import { apiFetch, readJSON } from "./client";

export interface AuthState {
  authenticated: boolean;
  setup_required: boolean;
  username?: string;
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
    (!value.authenticated ||
      ("username" in value &&
        typeof value.username === "string" &&
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
