import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  discordChannels,
  previewDiscord,
  saveDiscord,
  testDiscord,
  type DiscordChannel,
  type DiscordGuild,
  type DiscordStatus,
} from "../../api/integrations";
import { Button } from "../forms/Button";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";
import { TextInput } from "../forms/TextInput";
import { InlineMessage } from "../status/InlineMessage";

export interface ConnectionFormState {
  busy: boolean;
  canSave: boolean;
}

export function DiscordConnectionForm({
  current,
  formID,
  onStateChange,
  onSaved,
}: {
  current: DiscordStatus | null;
  formID: string;
  onStateChange: (state: ConnectionFormState) => void;
  onSaved: (status: DiscordStatus) => void;
}) {
  const { t } = useTranslation();
  const [replaceToken, setReplaceToken] = useState(!current);
  const [token, setToken] = useState("");
  const [guildID, setGuildID] = useState(current?.configuration.guild_id ?? "");
  const [lookup, setLookup] = useState<{
    token: string;
    guilds: DiscordGuild[];
    error: string | null;
    inviteURL: string;
  } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testOpen, setTestOpen] = useState(false);
  const [channels, setChannels] = useState<DiscordChannel[]>([]);
  const [loadingChannels, setLoadingChannels] = useState(false);
  const [channelID, setChannelID] = useState("");
  const [sent, setSent] = useState(false);
  const [serverRefresh, setServerRefresh] = useState(0);
  const alive = useRef(true);
  const pending = useRef(false);
  const suppliedToken = replaceToken ? token.trim() : "";
  const needsToken = !current || replaceToken;
  const lookupCurrent = lookup?.token === suppliedToken;
  const guilds = lookupCurrent ? lookup.guilds : [];
  const lookupError = lookupCurrent ? lookup.error : null;
  const inviteURL = lookupCurrent
    ? lookup.inviteURL
    : !needsToken
      ? current.invite_url
      : undefined;
  const loadingServers =
    (!needsToken || suppliedToken !== "") && !lookupCurrent;
  const changed =
    !current ||
    (needsToken && suppliedToken !== "") ||
    guildID !== current.configuration.guild_id;
  const busy = saving || testing;
  const canSave =
    changed &&
    !!guildID &&
    !loadingServers &&
    lookupCurrent &&
    !lookupError &&
    (!needsToken || suppliedToken !== "");

  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  useEffect(() => {
    const refresh = () => setServerRefresh((value) => value + 1);
    window.addEventListener("focus", refresh);
    return () => window.removeEventListener("focus", refresh);
  }, []);
  useEffect(() => {
    onStateChange({ busy, canSave });
  }, [busy, canSave, onStateChange]);

  useEffect(() => {
    const controller = new AbortController();
    if (needsToken && !suppliedToken) {
      return;
    }
    const timer = window.setTimeout(
      () => {
        void previewDiscord(suppliedToken, controller.signal)
          .then((preview) => {
            if (controller.signal.aborted) return;
            setLookup({
              token: suppliedToken,
              guilds: preview.guilds,
              error:
                preview.guild_error ??
                (preview.guilds.length ? null : t("integrations.noServers")),
              inviteURL: preview.invite_url,
            });
            setGuildID((selected) =>
              preview.guilds.some((guild) => guild.id === selected)
                ? selected
                : preview.guilds.length === 1
                  ? preview.guilds[0]!.id
                  : "",
            );
          })
          .catch((cause: unknown) => {
            if (!controller.signal.aborted)
              setLookup({
                token: suppliedToken,
                guilds: [],
                inviteURL: "",
                error:
                  cause instanceof Error
                    ? cause.message
                    : t("integrations.failed"),
              });
          });
      },
      needsToken ? 400 : 0,
    );
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [suppliedToken, needsToken, serverRefresh, t]);

  useEffect(() => {
    if (!testOpen || !current?.enabled) return;
    const controller = new AbortController();
    void discordChannels(controller.signal)
      .then((items) => {
        if (!controller.signal.aborted) {
          setChannels(items);
          setChannelID("");
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : t("integrations.failed"),
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingChannels(false);
      });
    return () => controller.abort();
  }, [testOpen, current?.enabled, t]);

  async function submit() {
    if (pending.current || !canSave) return;
    pending.current = true;
    setSaving(true);
    setError(null);
    try {
      const status = await saveDiscord({
        enabled: current?.enabled ?? true,
        guild_id: guildID,
        token: suppliedToken,
      });
      if (alive.current) onSaved(status);
    } catch (cause) {
      if (alive.current)
        setError(
          cause instanceof Error ? cause.message : t("integrations.failed"),
        );
    } finally {
      pending.current = false;
      if (alive.current) setSaving(false);
    }
  }

  const selected = channels.find((channel) => channel.id === channelID);
  return (
    <form
      id={formID}
      className="integration-form"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      {error && <InlineMessage role="alert">{error}</InlineMessage>}
      {current && (
        <div className="integration-identity">
          <strong>{current.configuration.bot_name}</strong>
          <span>{current.configuration.guild_name}</span>
        </div>
      )}
      {replaceToken ? (
        <FormField
          label={t("integrations.token")}
          description={t("integrations.tokenHelp")}
          labelAction={
            current
              ? {
                  label: t("common.cancel"),
                  disabled: busy,
                  onClick: () => {
                    setReplaceToken(false);
                    setToken("");
                    setGuildID(current.configuration.guild_id);
                  },
                }
              : undefined
          }
        >
          <TextInput
            type="password"
            autoComplete="new-password"
            spellCheck={false}
            disabled={busy}
            value={token}
            onChange={(event) => {
              setToken(event.target.value);
              setGuildID("");
            }}
          />
        </FormField>
      ) : (
        <FormField label={t("integrations.token")}>
          <Button
            type="button"
            size="small"
            aria-label={t("integrations.changeToken")}
            disabled={busy}
            onClick={() => setReplaceToken(true)}
          >
            {t("integrations.changeToken")}
          </Button>
        </FormField>
      )}
      {lookupError && <InlineMessage role="alert">{lookupError}</InlineMessage>}
      <FormField
        label={t("integrations.guild")}
        labelAction={
          inviteURL
            ? {
                label: t("integrations.inviteBot"),
                href: inviteURL,
                target: "_blank",
                rel: "noopener noreferrer",
              }
            : undefined
        }
        description={
          current
            ? t("integrations.required", {
                permissions: current.required_permissions.join(", "),
              })
            : undefined
        }
      >
        <Select
          value={guildID}
          disabled={busy || loadingServers || !guilds.length}
          onValueChange={setGuildID}
        >
          <option value="">
            {loadingServers
              ? t("integrations.loadingServers")
              : t("integrations.chooseServer")}
          </option>
          {guilds.map((guild) => (
            <option key={guild.id} value={guild.id}>
              {guild.name}
            </option>
          ))}
        </Select>
      </FormField>
      {current && (
        <details
          className="integration-test"
          onToggle={(event) => {
            const open = event.currentTarget.open;
            if (open && current.enabled) setLoadingChannels(true);
            setTestOpen(open);
          }}
        >
          <summary>{t("integrations.testTitle")}</summary>
          <div className="integration-test-fields">
            <p className="field-help">
              {t(
                current.enabled
                  ? "integrations.testHelp"
                  : "integrations.enableToTest",
              )}
            </p>
            <FormField
              label={t("integrations.channel")}
              description={
                selected && !selected.available
                  ? t("integrations.missing", {
                      permissions: selected.missing.join(", "),
                    })
                  : undefined
              }
            >
              <Select
                value={channelID}
                disabled={
                  busy || loadingChannels || !current.enabled || changed
                }
                onValueChange={(value) => {
                  setChannelID(value);
                  setSent(false);
                }}
              >
                <option value="">
                  {loadingChannels
                    ? t("common.loading")
                    : t("integrations.chooseChannel")}
                </option>
                {channels.map((channel) => (
                  <option key={channel.id} value={channel.id}>
                    {channel.available
                      ? `#${channel.name}`
                      : t("integrations.unavailableChannel", {
                          channel: channel.name,
                        })}
                  </option>
                ))}
              </Select>
            </FormField>
            <div>
              <Button
                type="button"
                disabled={
                  busy || changed || !current.enabled || !selected?.available
                }
                loading={testing}
                onClick={() => {
                  if (pending.current) return;
                  pending.current = true;
                  setTesting(true);
                  setError(null);
                  setSent(false);
                  void testDiscord(channelID)
                    .then(() => {
                      if (alive.current) setSent(true);
                    })
                    .catch((cause: unknown) => {
                      if (alive.current)
                        setError(
                          cause instanceof Error
                            ? cause.message
                            : t("integrations.failed"),
                        );
                    })
                    .finally(() => {
                      pending.current = false;
                      if (alive.current) setTesting(false);
                    });
                }}
              >
                {t("integrations.sendTest")}
              </Button>
            </div>
            {sent && (
              <InlineMessage tone="success">
                {t("integrations.sent")}
              </InlineMessage>
            )}
          </div>
        </details>
      )}
    </form>
  );
}
