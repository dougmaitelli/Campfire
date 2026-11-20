import { apiFetch, apiJSON, readJSON } from "./client";
import { isRecord, isEnumValue } from "../utils/validation";
import { PublicServerState } from "../contracts/domains";
import { StatusPagePolling } from "../config/policies";
import i18n from "../i18n";
import { isGameStateView, type GameStateView } from "./gameState";

export interface StatusPageSelection {
  server_id: string;
  name: string;
}
export interface StatusPageInput {
  title: string;
  slug: string;
  entries: StatusPageSelection[];
}
export interface StatusPage extends StatusPageInput {
  id: string;
}
export interface PublicStatusEntry {
  id: string;
  name: string;
  state: PublicServerState;
  observed_at: number;
  art_url?: string;
  game_address?: string;
  game_state?: GameStateView;
}
export interface PublicStatusPage {
  collected_at: number;
  title: string;
  entries: PublicStatusEntry[];
  poll_seconds: number;
}

function isSelection(v: unknown): v is StatusPageSelection {
  return (
    isRecord(v) && typeof v.server_id === "string" && typeof v.name === "string"
  );
}
function isStatusPage(v: unknown): v is StatusPage {
  return (
    isRecord(v) &&
    typeof v.id === "string" &&
    typeof v.title === "string" &&
    typeof v.slug === "string" &&
    Array.isArray(v.entries) &&
    v.entries.every(isSelection)
  );
}
function invalid(): never {
  throw new Error(i18n.t("statusPages.invalidResponse"));
}
export async function listStatusPages(
  signal: AbortSignal,
): Promise<StatusPage[]> {
  const body = await readJSON(
    await apiFetch("/api/v1/status-pages", { signal }),
  );
  if (!Array.isArray(body) || !body.every(isStatusPage)) return invalid();
  return body;
}
export async function saveStatusPage(
  id: string | undefined,
  input: StatusPageInput,
): Promise<StatusPage> {
  const body = await readJSON(
    await apiJSON(
      id
        ? `/api/v1/status-pages/${encodeURIComponent(id)}`
        : "/api/v1/status-pages",
      input,
      { method: id ? "PUT" : "POST" },
    ),
  );
  if (!isStatusPage(body)) return invalid();
  return body;
}
export async function deleteStatusPage(id: string): Promise<void> {
  const response = await apiFetch(
    `/api/v1/status-pages/${encodeURIComponent(id)}`,
    { method: "DELETE" },
  );
  if (!response.ok) await readJSON(response);
}
function isPublicEntry(v: unknown): v is PublicStatusEntry {
  return (
    isRecord(v) &&
    typeof v.id === "string" &&
    typeof v.name === "string" &&
    isEnumValue(PublicServerState, v.state) &&
    typeof v.observed_at === "number" &&
    Number.isSafeInteger(v.observed_at) &&
    v.observed_at >= 0 &&
    (v.art_url === undefined ||
      (typeof v.art_url === "string" && v.art_url.length <= 2048)) &&
    (v.game_address === undefined || typeof v.game_address === "string") &&
    (v.game_state === undefined || isGameStateView(v.game_state))
  );
}
export async function readPublicStatusPage(
  slug: string,
  signal: AbortSignal,
): Promise<PublicStatusPage> {
  // The public page neither sends credentials nor invokes session-expiry handling.
  const body = await readJSON(
    await fetch(`/api/v1/public/status-pages/${encodeURIComponent(slug)}`, {
      signal,
      credentials: "omit",
      redirect: "error",
      cache: "no-store",
      headers: { Accept: "application/json" },
    }),
  );
  if (
    !isRecord(body) ||
    typeof body.title !== "string" ||
    typeof body.collected_at !== "number" ||
    !Number.isSafeInteger(body.collected_at) ||
    body.collected_at < 0 ||
    !Array.isArray(body.entries) ||
    body.entries.length > 50 ||
    !body.entries.every(isPublicEntry) ||
    typeof body.poll_seconds !== "number" ||
    !Number.isSafeInteger(body.poll_seconds) ||
    body.poll_seconds <= 0 ||
    body.poll_seconds > StatusPagePolling.MaxSeconds
  )
    return invalid();
  return {
    title: body.title,
    collected_at: body.collected_at,
    entries: body.entries,
    poll_seconds: body.poll_seconds,
  };
}
export function statusPageURL(slug: string): string {
  return `/status/${encodeURIComponent(slug)}`;
}
