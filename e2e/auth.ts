import type { APIResponse } from "@playwright/test";
import type { AuthState } from "../web/src/api/auth";

// Login and session endpoints share AuthState. Mutation setup must explicitly
// require authentication and a token instead of asserting optional fields exist.
export async function readAuthenticatedSession(
  response: Pick<APIResponse, "ok" | "status" | "json">,
): Promise<AuthState & Required<Pick<AuthState, "csrf_token">>> {
  if (!response.ok()) {
    throw new Error(`Session request failed with HTTP ${response.status()}`);
  }
  const session = (await response.json()) as AuthState;
  if (
    !session ||
    session.authenticated !== true ||
    typeof session.csrf_token !== "string" ||
    !session.csrf_token
  ) {
    throw new Error("Expected an authenticated session with a CSRF token");
  }
  return { ...session, csrf_token: session.csrf_token };
}
