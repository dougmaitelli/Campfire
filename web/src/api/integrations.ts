import { apiFetch, apiJSON, readJSON } from "./client";
import { isRecord } from "../utils/validation";
import i18n from "../i18n";
import { isPermissionID, type PermissionID } from "./permissions";
import { RuntimeState } from "../contracts/domains";
import { isEnumValue } from "../utils/validation";
import type { PresentedGameField } from "../utils/gameFieldPresentation";
import { validGameFieldLabels, type GameFieldLabels } from "./gameFields";

export interface DiscordChannel {
  id: string;
  name: string;
  available: boolean;
  missing: string[];
}

export interface DiscordGuild {
  id: string;
  name: string;
}

export interface DiscordStatus {
  invite_url: string;
  configured: boolean;
  enabled: boolean;
  healthy: boolean;
  configuration: {
    bot_id: string;
    bot_name: string;
    guild_id: string;
    guild_name: string;
  };
  channels: DiscordChannel[];
  required_permissions: string[];
  checked_at: number;
  error: string;
}

function strings(value: unknown): value is string[] {
  return (
    Array.isArray(value) && value.every((item) => typeof item === "string")
  );
}

function channel(value: unknown): value is DiscordChannel {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.name === "string" &&
    typeof value.available === "boolean" &&
    strings(value.missing)
  );
}

function status(value: unknown): value is DiscordStatus {
  if (!isRecord(value)) return false;
  const configuration = value.configuration;
  return (
    isRecord(value) &&
    ["configured", "enabled", "healthy"].every(
      (key) => typeof value[key] === "boolean",
    ) &&
    isRecord(configuration) &&
    ["bot_id", "bot_name", "guild_id", "guild_name"].every(
      (key) => typeof configuration[key] === "string",
    ) &&
    Array.isArray(value.channels) &&
    value.channels.every(channel) &&
    strings(value.required_permissions) &&
    typeof value.checked_at === "number" &&
    Number.isSafeInteger(value.checked_at) &&
    typeof value.error === "string" &&
    typeof value.invite_url === "string"
  );
}

function unexpected(): never {
  throw new Error(i18n.t("integrations.unexpectedResponse"));
}

export async function discordStatus(
  signal?: AbortSignal,
): Promise<DiscordStatus> {
  const value = await readJSON(
    await apiFetch("/api/v1/integrations/discord", signal ? { signal } : {}),
  );
  return status(value) ? value : unexpected();
}

export async function previewDiscord(
  token: string,
  signal?: AbortSignal,
): Promise<{
  bot: { id: string; username: string };
  guilds: DiscordGuild[];
  invite_url: string;
  guild_error?: string | undefined;
}> {
  const value = await readJSON(
    await apiJSON(
      "/api/v1/integrations/discord/preview",
      { token },
      signal ? { signal } : {},
    ),
  );
  if (
    !isRecord(value) ||
    !isRecord(value.bot) ||
    typeof value.bot.id !== "string" ||
    typeof value.bot.username !== "string" ||
    typeof value.invite_url !== "string" ||
    (value.guild_error !== undefined &&
      typeof value.guild_error !== "string") ||
    !Array.isArray(value.guilds) ||
    !value.guilds.every(
      (item) =>
        isRecord(item) &&
        typeof item.id === "string" &&
        typeof item.name === "string",
    )
  )
    return unexpected();
  return {
    bot: { id: value.bot.id, username: value.bot.username },
    guilds: value.guilds as DiscordGuild[],
    invite_url: value.invite_url,
    guild_error: value.guild_error,
  };
}

export async function saveDiscord(input: {
  enabled: boolean;
  guild_id: string;
  token: string;
}): Promise<DiscordStatus> {
  const value = await readJSON(
    await apiJSON("/api/v1/integrations/discord", input, { method: "PUT" }),
  );
  return status(value) ? value : unexpected();
}

export async function removeDiscord(): Promise<void> {
  await apiJSON("/api/v1/integrations/discord", {}, { method: "DELETE" });
}

export async function discordChannels(
  signal?: AbortSignal,
): Promise<DiscordChannel[]> {
  const value = await readJSON(
    await apiFetch(
      "/api/v1/integrations/discord/channels",
      signal ? { signal } : {},
    ),
  );
  return Array.isArray(value) && value.every(channel) ? value : unexpected();
}

export async function testDiscord(channelID: string): Promise<void> {
  await apiJSON("/api/v1/integrations/discord/test", { channel_id: channelID });
}

export type NotificationCategory =
  "game" | "power" | "console_alert" | "backup";

export interface ServerIntegrationInput {
  integration_id: string;
  enabled: boolean;
  channel_id: string;
  subscriptions: {
    category: NotificationCategory;
    rules?: { type: string; fields?: Record<string, string> }[];
  }[];
  card: { enabled: boolean; fields: string[] };
}

export interface IntegrationConnection {
  id: string;
  provider: string;
  name: string;
  enabled: boolean;
}

export interface ServerIntegrationSettings extends ServerIntegrationInput {
  id: string;
  configured: boolean;
  available: boolean;
  guild_name: string;
  sources: { id: NotificationCategory; permission: PermissionID }[];
  card_status: IntegrationCardStatus;
}

export interface ServerIntegrationList {
  field_labels?: GameFieldLabels;
  connections: IntegrationConnection[];
  configured: boolean;
  available: boolean;
  guild_name: string;
  sources: ServerIntegrationSettings["sources"];
  items: ServerIntegrationSettings[];
}

export interface IntegrationCardStatus {
  state: "disabled" | "pending" | "published" | "uncertain" | "failed";
  error: string;
  next_attempt_at: number;
  can_retry: boolean;
}

function category(value: unknown): value is NotificationCategory {
  return ["game", "power", "console_alert", "backup"].includes(value as string);
}

function serverSettings(value: unknown): value is ServerIntegrationSettings {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.integration_id === "string" &&
    typeof value.enabled === "boolean" &&
    typeof value.channel_id === "string" &&
    typeof value.configured === "boolean" &&
    typeof value.available === "boolean" &&
    typeof value.guild_name === "string" &&
    isRecord(value.card_status) &&
    ["disabled", "pending", "published", "uncertain", "failed"].includes(
      value.card_status.state as string,
    ) &&
    typeof value.card_status.error === "string" &&
    typeof value.card_status.can_retry === "boolean" &&
    Number.isSafeInteger(value.card_status.next_attempt_at) &&
    Array.isArray(value.subscriptions) &&
    value.subscriptions.every(
      (item: unknown) =>
        isRecord(item) &&
        category(item.category) &&
        (item.rules === undefined ||
          (Array.isArray(item.rules) &&
            item.rules.every(
              (rule: unknown) =>
                isRecord(rule) &&
                typeof rule.type === "string" &&
                (rule.fields === undefined ||
                  (isRecord(rule.fields) &&
                    Object.values(rule.fields).every(
                      (field) => typeof field === "string",
                    ))),
            ))),
    ) &&
    isRecord(value.card) &&
    typeof value.card.enabled === "boolean" &&
    strings(value.card.fields) &&
    Array.isArray(value.sources) &&
    value.sources.every(
      (item: unknown) =>
        isRecord(item) && category(item.id) && isPermissionID(item.permission),
    )
  );
}

function serverIntegrationURL(serverID: string) {
  return `/api/v1/servers/${encodeURIComponent(serverID)}/integrations/discord`;
}

function bindingURL(serverID: string, bindingID: string) {
  return `${serverIntegrationURL(serverID)}/${encodeURIComponent(bindingID)}`;
}

export async function listServerDiscord(
  serverID: string,
  signal?: AbortSignal,
): Promise<ServerIntegrationList> {
  const value = await readJSON(
    await apiFetch(serverIntegrationURL(serverID), signal ? { signal } : {}),
  );
  if (
    !isRecord(value) ||
    typeof value.configured !== "boolean" ||
    typeof value.available !== "boolean" ||
    typeof value.guild_name !== "string" ||
    (value.field_labels !== undefined &&
      (!isRecord(value.field_labels) ||
        !validGameFieldLabels(value.field_labels, value.field_labels))) ||
    !Array.isArray(value.connections) ||
    !value.connections.every(
      (connection: unknown) =>
        isRecord(connection) &&
        typeof connection.id === "string" &&
        typeof connection.provider === "string" &&
        typeof connection.name === "string" &&
        typeof connection.enabled === "boolean",
    ) ||
    !Array.isArray(value.sources) ||
    !value.sources.every(
      (item: unknown) =>
        isRecord(item) && category(item.id) && isPermissionID(item.permission),
    ) ||
    !Array.isArray(value.items) ||
    !value.items.every(serverSettings)
  )
    return unexpected();
  return value as unknown as ServerIntegrationList;
}

export async function serverDiscordSettings(
  serverID: string,
  bindingID: string,
  signal?: AbortSignal,
): Promise<ServerIntegrationSettings> {
  const value = await readJSON(
    await apiFetch(bindingURL(serverID, bindingID), signal ? { signal } : {}),
  );
  return serverSettings(value) ? value : unexpected();
}

export async function serverDiscordChannels(
  serverID: string,
  connectionID: string,
  signal?: AbortSignal,
): Promise<DiscordChannel[]> {
  const value = await readJSON(
    await apiFetch(
      `${serverIntegrationURL(serverID)}/channels?${new URLSearchParams({ integration_id: connectionID })}`,
      signal ? { signal } : {},
    ),
  );
  return Array.isArray(value) && value.every(channel) ? value : unexpected();
}

export interface ServerIntegrationChanges {
  upserts: { id: string; settings: ServerIntegrationInput }[];
  remove_ids: string[];
}

export async function saveServerIntegrationChanges(
  serverID: string,
  changes: ServerIntegrationChanges,
): Promise<ServerIntegrationSettings[]> {
  const value = await readJSON(
    await apiJSON(serverIntegrationURL(serverID), changes, { method: "PUT" }),
  );
  return Array.isArray(value) && value.every(serverSettings)
    ? value
    : unexpected();
}

export async function retryIntegrationCard(
  serverID: string,
  bindingID: string,
): Promise<void> {
  await apiJSON(`${bindingURL(serverID, bindingID)}/card/retry`, {});
}

export interface RenderedIntegrationCard {
  embeds: {
    title: string;
    fields: { name: string; value: string }[];
    footer: { text: string };
  }[];
}

function renderedIntegrationCard(
  value: unknown,
): value is RenderedIntegrationCard {
  return (
    isRecord(value) &&
    Array.isArray(value.embeds) &&
    value.embeds.length > 0 &&
    value.embeds.every(
      (embed: unknown) =>
        isRecord(embed) &&
        typeof embed.title === "string" &&
        isRecord(embed.footer) &&
        typeof embed.footer.text === "string" &&
        Array.isArray(embed.fields) &&
        embed.fields.every(
          (field: unknown) =>
            isRecord(field) &&
            typeof field.name === "string" &&
            typeof field.value === "string",
        ),
    )
  );
}

export interface IntegrationCard {
  name: string;
  state: RuntimeState;
  addresses: string[];
  expires_at: number;
  fields: PresentedGameField[];
  preview: RenderedIntegrationCard;
  expired_preview: RenderedIntegrationCard;
}

export async function previewIntegrationCard(
  serverID: string,
  fields: string[],
  signal: AbortSignal,
): Promise<IntegrationCard> {
  const query = new URLSearchParams();
  for (const field of fields) query.append("field", field);
  const value = await readJSON(
    await apiFetch(`${serverIntegrationURL(serverID)}/card?${query}`, {
      signal,
    }),
  );
  if (
    !isRecord(value) ||
    !renderedIntegrationCard(value.preview) ||
    !renderedIntegrationCard(value.expired_preview) ||
    typeof value.name !== "string" ||
    !isEnumValue(RuntimeState, value.state) ||
    !strings(value.addresses) ||
    !Number.isSafeInteger(value.expires_at) ||
    (value.expires_at as number) < 0 ||
    !Array.isArray(value.fields) ||
    !value.fields.every(
      (field: unknown) =>
        isRecord(field) &&
        typeof field.id === "string" &&
        typeof field.name === "string" &&
        typeof field.value === "string" &&
        typeof field.available === "boolean",
    )
  )
    return unexpected();
  return value as unknown as IntegrationCard;
}
