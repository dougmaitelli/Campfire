import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  listServerDiscord,
  saveServerIntegrationChanges,
  serverDiscordChannels,
  type DiscordChannel,
  type ServerIntegrationInput,
  type ServerIntegrationList,
  type ServerIntegrationSettings,
} from "../api/integrations";
import type { Server } from "../api/servers";
import { IntegrationLimits } from "../config/policies";
import type { GameEventRuleDraft } from "../components/servers/GameEventSubscriptionEditor";
import { needsIntegrationPublishingCheck } from "../utils/integrationSettings";
import { useMonitoring } from "./useMonitoring";

export interface ServerIntegrationDraft {
  key: string;
  value: ServerIntegrationInput;
  rules: GameEventRuleDraft[];
}

function editable(value: ServerIntegrationInput): ServerIntegrationInput {
  return {
    integration_id: value.integration_id,
    enabled: value.enabled,
    channel_id: value.channel_id,
    subscriptions: value.subscriptions,
    card: value.card,
  };
}

function initialRules(value: ServerIntegrationInput): GameEventRuleDraft[] {
  const game = value.subscriptions.find((item) => item.category === "game");
  return game?.rules?.length
    ? game.rules.map((rule) => ({
        type: rule.type,
        fields: Object.entries(rule.fields ?? {}).map(([name, value]) => ({
          name,
          value,
        })),
      }))
    : [{ type: "", fields: [] }];
}

// Drafts live outside the current wizard step so navigation and Cancel preserve
// the same save boundary as server configuration, access, and backup policy.
export function useServerIntegrations(server: Server | null, active: boolean) {
  const { t } = useTranslation();
  const serverID = server?.id;
  const monitoring = useMonitoring(active && server?.game_integration === true);
  const game = monitoring.snapshot?.servers.find((item) => item.id === serverID)
    ?.game?.state;
  const [availableFields, setAvailableFields] = useState<string[]>([]);
  const reportedFields = [
    ...(game?.players !== undefined ? ["players"] : []),
    ...(game?.capacity !== undefined ? ["capacity"] : []),
    ...Object.keys(game?.fields ?? {}).map((field) => `field.${field}`),
  ];
  const unseen = reportedFields.filter(
    (field) => !availableFields.includes(field),
  );
  if (unseen.length) setAvailableFields([...availableFields, ...unseen]);
  const [overview, setOverview] = useState<ServerIntegrationList | null>(null);
  const [items, setItems] = useState<ServerIntegrationDraft[]>([]);
  const [editing, setEditing] = useState<string | null>(null);
  const [loading, setLoading] = useState(active);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [channels, setChannels] = useState<Record<string, DiscordChannel[]>>(
    {},
  );
  const [channelErrors, setChannelErrors] = useState<Record<string, string>>(
    {},
  );
  const [saved, setSaved] = useState(
    new Map<string, ServerIntegrationSettings>(),
  );

  useEffect(() => {
    if (!active || !serverID) return;
    const controller = new AbortController();
    void listServerDiscord(serverID, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        setSaved(new Map(result.items.map((item) => [item.id, item])));
        setOverview(result);
        setItems(
          result.items.map((item) => ({
            key: item.id,
            value: editable(item),
            rules: initialRules(item),
          })),
        );
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : t("integrations.failed"),
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [active, serverID, attempt, t]);

  useEffect(() => {
    if (!active || !serverID) return;
    const controller = new AbortController();
    for (const connection of overview?.connections ?? []) {
      if (!connection.enabled) continue;
      void serverDiscordChannels(serverID, connection.id, controller.signal)
        .then((result) => {
          if (!controller.signal.aborted)
            setChannels((current) => ({ ...current, [connection.id]: result }));
        })
        .catch((cause: unknown) => {
          if (!controller.signal.aborted)
            setChannelErrors((current) => ({
              ...current,
              [connection.id]:
                cause instanceof Error
                  ? cause.message
                  : t("integrations.failed"),
            }));
        });
    }
    return () => controller.abort();
  }, [active, serverID, overview?.connections, t]);

  function changed(item: ServerIntegrationDraft, baseline = saved) {
    const original = baseline.get(item.key);
    return (
      !original ||
      JSON.stringify(item.value) !== JSON.stringify(editable(original))
    );
  }
  const dirty =
    active &&
    (items.length !== saved.size ||
      items.some(
        (item) =>
          changed(item) ||
          (item.value.subscriptions.some(
            (source) => source.category === "game",
          ) &&
            JSON.stringify(item.rules) !==
              JSON.stringify(initialRules(item.value))),
      ));

  function validate() {
    if (!dirty) return null;
    if (loading) return t("common.loading");
    if (error) return error;
    for (const item of items) {
      const game = item.value.subscriptions.some(
        (source) => source.category === "game",
      );
      if (
        game &&
        item.rules.some(
          (rule) =>
            rule.type === null || rule.fields.some((field) => !field.name),
        )
      )
        return t("serverIntegrations.completeFilters");
      if (!changed(item)) continue;
      const input = item.value;
      if (input.card.fields.length > IntegrationLimits.CardFields)
        return t("serverIntegrations.tooManyCardFields", {
          count: IntegrationLimits.CardFields,
        });
      if (!input.integration_id || !input.channel_id)
        return t("serverIntegrations.completeSettings");
      if (
        items.some(
          (other) =>
            other.key !== item.key &&
            other.value.integration_id === input.integration_id &&
            other.value.channel_id === input.channel_id,
        )
      )
        return t("serverIntegrations.duplicateChannel");
      const connection = overview?.connections.find(
        (connection) => connection.id === input.integration_id,
      );
      const channel = channels[input.integration_id]?.find(
        (channel) => channel.id === input.channel_id,
      );
      if (
        needsIntegrationPublishingCheck(saved.get(item.key) ?? null, input) &&
        (!connection?.enabled || !channel?.available)
      )
        return t("serverIntegrations.completeSettings");
      if (
        input.enabled &&
        input.subscriptions.some((subscription) => {
          const source = overview?.sources.find(
            (source) => source.id === subscription.category,
          );
          return !source || !server?.permissions.includes(source.permission);
        })
      )
        return t("serverIntegrations.completeSettings");
    }
    return null;
  }

  async function save() {
    if (!active || !serverID || !dirty) return;
    const invalid = validate();
    if (invalid) throw new Error(invalid);
    const upserts = items
      .filter((item) => changed(item))
      .map((item) => ({ id: item.key, settings: item.value }));
    const removeIDs = [...saved.keys()].filter(
      (key) => !items.some((item) => item.key === key),
    );
    if (!upserts.length && !removeIDs.length) return;
    // IDs are chosen when drafts are added and remain identical after a lost
    // response. The server atomically upserts them and ignores completed deletes.
    const result = await saveServerIntegrationChanges(serverID, {
      upserts,
      remove_ids: removeIDs,
    });
    const baseline = new Map(saved);
    for (const id of removeIDs) baseline.delete(id);
    for (const value of result) baseline.set(value.id, value);
    setSaved(baseline);
  }

  return {
    overview,
    availableFields,
    items,
    editing,
    setEditing,
    channels,
    channelErrors,
    loading,
    error,
    dirty,
    validate,
    save,
    original: (key: string) => saved.get(key) ?? null,
    retry: () => {
      setError(null);
      setLoading(true);
      setAttempt((value) => value + 1);
    },
    add: () => {
      const key = crypto.randomUUID();
      const value: ServerIntegrationInput = {
        integration_id:
          overview?.connections.length === 1 ? overview.connections[0]!.id : "",
        enabled: true,
        channel_id: "",
        subscriptions: [],
        card: { enabled: false, fields: [] },
      };
      setItems((current) => [
        ...current,
        { key, value, rules: initialRules(value) },
      ]);
      setEditing(key);
    },
    remove: (key: string) => {
      setItems((current) => current.filter((item) => item.key !== key));
      setEditing(null);
    },
    update: (
      key: string,
      value: ServerIntegrationInput,
      rules?: GameEventRuleDraft[],
    ) => {
      setItems((current) =>
        current.map((item) => {
          if (item.key !== key) return item;
          const addedGame =
            !item.value.subscriptions.some(
              (source) => source.category === "game",
            ) &&
            value.subscriptions.some((source) => source.category === "game");
          return {
            ...item,
            value,
            rules: rules ?? (addedGame ? initialRules(value) : item.rules),
          };
        }),
      );
    },
  };
}

export type ServerIntegrationsState = ReturnType<typeof useServerIntegrations>;
