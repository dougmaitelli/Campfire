import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  discordStatus,
  removeDiscord,
  saveDiscord,
  type DiscordStatus,
} from "../api/integrations";
import { Button } from "../components/forms/Button";
import { FormField } from "../components/forms/FormField";
import { Select } from "../components/forms/Select";
import { ToggleSwitch } from "../components/forms/ToggleSwitch";
import {
  DiscordConnectionForm,
  type ConnectionFormState,
} from "../components/integrations/DiscordConnectionForm";
import { Drawer } from "../components/layout/Drawer";
import { InlineMessage } from "../components/status/InlineMessage";
import { Table, TableRow } from "../components/table/Table";
import { useConfirmation } from "../hooks/useConfirmation";
import "./integrations.css";

export function IntegrationsDrawer({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const confirmation = useConfirmation();
  const formID = useId();
  const [current, setCurrent] = useState<DiscordStatus | null>(null);
  const [mode, setMode] = useState<"list" | "add" | "edit">("list");
  const [provider, setProvider] = useState("");
  const [loading, setLoading] = useState(true);
  const [working, setWorking] = useState(false);
  const [form, setForm] = useState<ConnectionFormState>({
    busy: false,
    canSave: false,
  });
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  const alive = useRef(true);
  const configured = current?.configured === true;
  const busy = working || form.busy;

  useEffect(() => {
    const controller = new AbortController();
    alive.current = true;
    void discordStatus(controller.signal)
      .then((status) => {
        if (!controller.signal.aborted) setCurrent(status);
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
    return () => {
      alive.current = false;
      controller.abort();
    };
  }, [t]);

  function showList() {
    setMode("list");
    setProvider("");
    setForm({ busy: false, canSave: false });
    setError(null);
  }
  async function perform(action: () => Promise<void>) {
    if (pending.current) return;
    pending.current = true;
    setWorking(true);
    setError(null);
    try {
      await action();
    } catch (cause) {
      if (alive.current)
        setError(
          cause instanceof Error ? cause.message : t("integrations.failed"),
        );
    } finally {
      pending.current = false;
      if (alive.current) setWorking(false);
    }
  }
  async function remove() {
    if (
      busy ||
      !(await confirmation.confirm({
        title: t("integrations.removeTitle"),
        description: t("integrations.removeHelp"),
        confirmLabel: t("integrations.remove"),
        danger: true,
      }))
    )
      return;
    await perform(async () => {
      await removeDiscord();
      const status = await discordStatus();
      if (alive.current) {
        setCurrent(status);
        showList();
      }
    });
  }

  return (
    <>
      <Drawer
        title={t("integrations.title")}
        description={t("integrations.description")}
        busy={busy}
        onClose={onClose}
        footerLeft={
          <Button
            disabled={busy}
            onClick={mode === "list" ? onClose : showList}
          >
            {t(mode === "list" ? "common.close" : "common.back")}
          </Button>
        }
        footerRight={
          mode === "list" ? null : (
            <>
              {mode === "edit" && (
                <Button
                  variant="danger"
                  disabled={busy}
                  onClick={() => {
                    void remove();
                  }}
                >
                  {t("integrations.remove")}
                </Button>
              )}
              {provider === "discord" && (
                <Button
                  variant="primary"
                  type="submit"
                  form={formID}
                  disabled={busy || !form.canSave}
                  loading={form.busy}
                >
                  {t(
                    mode === "add" ? "integrations.add" : "common.saveChanges",
                  )}
                </Button>
              )}
            </>
          )
        }
      >
        <div className="panel-content">
          {error && <InlineMessage role="alert">{error}</InlineMessage>}
          {loading ? (
            <p>{t("common.loading")}</p>
          ) : mode === "list" ? (
            <>
              <div className="integration-list-heading">
                <h3>{t("integrations.connections")}</h3>
                <Button
                  disabled={busy || configured}
                  onClick={() => setMode("add")}
                >
                  {t("integrations.add")}
                </Button>
              </div>
              {configured ? (
                <Table aria-label={t("integrations.connections")}>
                  <thead>
                    <tr>
                      <th scope="col">{t("integrations.provider")}</th>
                      <th scope="col">{t("integrations.guild")}</th>
                      <th scope="col" className="table-fit">
                        {t("common.enabled")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    <TableRow
                      onOpen={
                        busy
                          ? undefined
                          : () => {
                              setProvider("discord");
                              setMode("edit");
                            }
                      }
                    >
                      <td>
                        <button
                          type="button"
                          className="table-row-trigger"
                          disabled={busy}
                          aria-label={t("integrations.editDiscord")}
                          onClick={() => {
                            setProvider("discord");
                            setMode("edit");
                          }}
                        >
                          {t("integrations.discord")}
                        </button>
                        {current.error && (
                          <InlineMessage tone="warning">
                            {current.error}
                          </InlineMessage>
                        )}
                      </td>
                      <td>
                        {current.configuration.guild_name}
                        <span className="row-description">
                          {current.configuration.bot_name}
                        </span>
                      </td>
                      <td className="table-fit">
                        <ToggleSwitch
                          aria-label={t("integrations.enableConnection")}
                          checked={current.enabled}
                          disabled={busy}
                          onChange={(event) => {
                            const enabled = event.target.checked;
                            const previous = current;
                            setCurrent({ ...current, enabled });
                            void perform(async () => {
                              try {
                                const status = await saveDiscord({
                                  enabled,
                                  guild_id: previous.configuration.guild_id,
                                  token: "",
                                });
                                if (alive.current) setCurrent(status);
                              } catch (cause) {
                                if (alive.current) setCurrent(previous);
                                throw cause;
                              }
                            });
                          }}
                        />
                      </td>
                    </TableRow>
                  </tbody>
                </Table>
              ) : (
                <p className="field-help">{t("integrations.empty")}</p>
              )}
            </>
          ) : (
            <>
              <h3>
                {t(
                  mode === "add"
                    ? "integrations.add"
                    : "integrations.editDiscord",
                )}
              </h3>
              {mode === "add" && (
                <FormField label={t("integrations.provider")}>
                  <Select
                    value={provider}
                    onValueChange={setProvider}
                    disabled={busy}
                  >
                    <option value="">{t("integrations.chooseProvider")}</option>
                    <option value="discord">{t("integrations.discord")}</option>
                  </Select>
                </FormField>
              )}
              {provider === "discord" && (
                <DiscordConnectionForm
                  current={mode === "edit" ? current : null}
                  formID={formID}
                  onStateChange={setForm}
                  onSaved={(status) => {
                    setCurrent(status);
                    showList();
                  }}
                />
              )}
            </>
          )}
        </div>
      </Drawer>
      {confirmation.dialog}
    </>
  );
}
