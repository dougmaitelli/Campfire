import i18n from "../i18n";
import { formatErrorMessage } from "../utils/errors";

let csrfToken = "";

export const API_REQUEST_TIMEOUT_MS = 35000;

export function setCSRFToken(token: string) {
  csrfToken = token;
}

export async function apiFetch(
  url: string,
  init: RequestInit = {},
): Promise<Response> {
  const headers = new Headers(init.headers);
  if (!headers.has("Accept")) headers.set("Accept", "application/json");
  if (
    !["GET", "HEAD", "OPTIONS"].includes((init.method ?? "GET").toUpperCase())
  )
    headers.set("X-CSRF-Token", csrfToken);
  const response = await fetch(url, {
    ...init,
    credentials: "same-origin",
    cache: "no-store",
    headers,
    signal: init.signal ?? AbortSignal.timeout(API_REQUEST_TIMEOUT_MS),
  });
  if (response.status === 401) {
    csrfToken = "";
    window.dispatchEvent(new Event("campfire:unauthenticated"));
  }
  return response;
}

export async function readJSON(response: Response): Promise<unknown> {
  let body: unknown;
  try {
    body = await response.json();
  } catch (cause) {
    if (!(cause instanceof SyntaxError)) throw cause;
    const path = response.url ? new URL(response.url).pathname : "the API";
    const contentType =
      response.headers.get("Content-Type")?.split(";")[0] ??
      i18n.t("client.missingContentType");
    throw new Error(
      i18n.t(
        "client.expectedJSONFromButReceivedAnUnexpectedResponseHTTPCheck",
        {
          path: path,
          responseStatus: response.status,
          contentType: contentType,
        },
      ),
      { cause },
    );
  }
  if (!response.ok) {
    const message =
      typeof body === "object" &&
      body !== null &&
      "error" in body &&
      typeof body.error === "string"
        ? body.error
        : i18n.t("client.unableToCompleteTheRequest");
    throw new Error(formatErrorMessage(message));
  }
  return body;
}
