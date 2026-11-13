import i18n from "../i18n";
import { formatErrorMessage } from "../utils/errors";
import { RequestTimeout } from "../config/policies";
import { BrowserEvent } from "../config/browser";

let csrfToken = "";

export const API_REQUEST_TIMEOUT_MS = RequestTimeout.Default;
export const API_READ_TIMEOUT_MS = RequestTimeout.Read;

export class APIError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = "APIError";
  }
}

export class ConnectionError extends Error {
  constructor(cause: unknown) {
    super(i18n.t("connection.requestFailed"), { cause });
    this.name = "ConnectionError";
  }
}

function requestFailure(cause: unknown): unknown {
  return (cause instanceof Error || cause instanceof DOMException) &&
    ["TypeError", "NetworkError", "TimeoutError"].includes(cause.name)
    ? new ConnectionError(cause)
    : cause;
}

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
  let response: Response;
  try {
    response = await fetch(url, {
      ...init,
      credentials: "same-origin",
      cache: "no-store",
      headers,
      signal: init.signal ?? AbortSignal.timeout(API_REQUEST_TIMEOUT_MS),
    });
  } catch (cause) {
    throw requestFailure(cause);
  }
  if (response.status === 401) {
    csrfToken = "";
    window.dispatchEvent(new Event(BrowserEvent.Unauthenticated));
  }
  return response;
}

export function apiJSON(
  url: string,
  body: unknown,
  init: Omit<RequestInit, "body"> = {},
): Promise<Response> {
  return apiFetch(url, jsonRequest(body, { method: "POST", ...init }));
}

export function jsonRequest(
  body: unknown,
  init: Omit<RequestInit, "body"> = {},
): RequestInit {
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json");
  return {
    ...init,
    headers,
    body: JSON.stringify(body),
  };
}

export async function readJSON(response: Response): Promise<unknown> {
  let body: unknown;
  try {
    body = await response.json();
  } catch (cause) {
    if (!(cause instanceof SyntaxError)) throw requestFailure(cause);
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
    throw new APIError(formatErrorMessage(message), response.status);
  }
  return body;
}
