import { formatErrorMessage } from "../utils/errors";

let csrfToken = "";

export function setCSRFToken(token: string) {
  csrfToken = token;
}

export async function apiFetch(
  url: string,
  init: RequestInit = {},
): Promise<Response> {
  const headers = new Headers(init.headers);
  if (
    !["GET", "HEAD", "OPTIONS"].includes((init.method ?? "GET").toUpperCase())
  )
    headers.set("X-CSRF-Token", csrfToken);
  const response = await fetch(url, {
    ...init,
    credentials: "same-origin",
    cache: "no-store",
    headers,
    signal: init.signal ?? AbortSignal.timeout(35000),
  });
  if (response.status === 401) {
    csrfToken = "";
    window.dispatchEvent(new Event("campfire:unauthenticated"));
  }
  return response;
}

export async function readJSON(response: Response): Promise<unknown> {
  const body: unknown = await response.json();
  if (!response.ok) {
    const message =
      typeof body === "object" &&
      body !== null &&
      "error" in body &&
      typeof body.error === "string"
        ? body.error
        : "Unable to complete the request.";
    throw new Error(formatErrorMessage(message));
  }
  return body;
}
