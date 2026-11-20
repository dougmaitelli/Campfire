import { useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { listServers } from "../api/servers";
import {
  saveStatusPage,
  type StatusPage,
  type StatusPageSelection,
} from "../api/statusPages";
import { useCollection } from "../hooks/useCollection";
import { Drawer } from "../components/layout/Drawer";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";
import { EditableList } from "../components/forms/EditableList";
import { Fieldset } from "../components/forms/Fieldset";
import { FormField } from "../components/forms/FormField";
import { TextInput } from "../components/forms/TextInput";
import { Select } from "../components/forms/Select";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";

type Entry = StatusPageSelection & { key: number };

export function StatusPageDrawer({
  page,
  onClose,
  onSaved,
  onDelete,
  deleting,
  deleteError,
}: {
  page: StatusPage | null;
  onClose: () => void;
  onSaved: (page: StatusPage) => void;
  onDelete: (() => void) | undefined;
  deleting: boolean;
  deleteError: string | null;
}) {
  const { t } = useTranslation();
  const formID = useId();
  const servers = useCollection(listServers);
  const [title, setTitle] = useState(page?.title ?? "");
  const [slug, setSlug] = useState(page?.slug ?? "");
  const [entries, setEntries] = useState<Entry[]>(
    () => page?.entries.map((entry, key) => ({ ...entry, key })) ?? [],
  );
  const nextKey = useRef(entries.length);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  function select(key: number, serverID: string) {
    const server = servers.items.find((item) => item.id === serverID);
    if (!server) return;
    setEntries((current) =>
      current.map((entry) =>
        entry.key === key
          ? { ...entry, server_id: serverID, name: server.name }
          : entry,
      ),
    );
  }
  function rowLabel(index: number) {
    const name = entries[index]?.name;
    return name?.trim()
      ? name
      : t("statusPages.serverEntry", { number: index + 1 });
  }
  function move(from: number, to: number) {
    setEntries((current) => {
      if (to < 0 || to >= current.length) return current;
      const next = [...current];
      const [entry] = next.splice(from, 1);
      if (!entry) return current;
      next.splice(to, 0, entry);
      return next;
    });
  }
  async function save() {
    if (pending.current || deleting) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      onSaved(
        await saveStatusPage(page?.id, {
          title,
          slug,
          entries: entries.map(({ server_id, name }) => ({ server_id, name })),
        }),
      );
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("statusPages.failed"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={t(page ? "statusPages.edit" : "statusPages.create")}
      busy={busy || deleting}
      onClose={onClose}
      footerLeft={
        <Button onClick={onClose} disabled={busy || deleting}>
          {t("common.cancel")}
        </Button>
      }
      footerRight={
        <>
          <DrawerDeleteButton
            onDelete={onDelete}
            disabled={busy}
            deleting={deleting}
          />
          <Button
            type="submit"
            form={formID}
            variant="primary"
            disabled={
              busy || deleting || entries.some((entry) => !entry.server_id)
            }
          >
            {t(page ? "common.saveChanges" : "statusPages.createPublic")}
          </Button>
        </>
      }
    >
      <form
        className="panel-content"
        id={formID}
        onSubmit={(event) => {
          event.preventDefault();
          void save();
        }}
      >
        <Fieldset disabled={busy || deleting}>
          <FormField label={t("statusPages.title")}>
            <TextInput
              required
              maxLength={100}
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
          </FormField>
          <FormField label={t("statusPages.slug")}>
            <TextInput
              required
              minLength={3}
              maxLength={64}
              pattern={"[a-z0-9][a-z0-9\\-]{1,62}[a-z0-9]"}
              value={slug}
              onChange={(event) => setSlug(event.target.value)}
            />
          </FormField>
          <p className="field-help">{t("statusPages.slugHelp")}</p>
          {servers.error && (
            <InlineMessage role="alert">
              {servers.error}
              <Button type="button" onClick={() => void servers.refresh()}>
                {t("common.tryAgain")}
              </Button>
            </InlineMessage>
          )}
          <EditableList
            items={entries}
            itemKey={(entry) => entry.key}
            reorderable
            onMove={move}
            rowLabel={rowLabel}
            removeLabel={(index) =>
              t("statusPages.remove", { name: rowLabel(index) })
            }
            addLabel={t("statusPages.addServer")}
            maxItems={50}
            addDisabled={
              servers.loading ||
              entries.some((entry) => !entry.server_id) ||
              servers.items.every((server) =>
                entries.some((entry) => entry.server_id === server.id),
              )
            }
            onAdd={() => {
              const key = nextKey.current++;
              setEntries((current) => [
                ...current,
                { key, server_id: "", name: "" },
              ]);
            }}
            onRemove={(index) =>
              setEntries((current) =>
                current.filter((_, position) => position !== index),
              )
            }
          >
            {(entry, index) => (
              <>
                <FormField
                  label={t("statusPages.serverEntry", { number: index + 1 })}
                >
                  <Select
                    value={entry.server_id}
                    onValueChange={(id) => select(entry.key, id)}
                    disabled={servers.loading}
                  >
                    <option value="">{t("statusPages.chooseServer")}</option>
                    {entry.server_id &&
                      !servers.items.some(
                        (server) => server.id === entry.server_id,
                      ) && (
                        <option value={entry.server_id}>{entry.name}</option>
                      )}
                    {servers.items
                      .filter(
                        (server) =>
                          server.id === entry.server_id ||
                          !entries.some(
                            (selected) => selected.server_id === server.id,
                          ),
                      )
                      .map((server) => (
                        <option key={server.id} value={server.id}>
                          {server.name}
                        </option>
                      ))}
                  </Select>
                </FormField>
                <FormField
                  label={t("statusPages.publicName", {
                    server:
                      servers.items.find(
                        (server) => server.id === entry.server_id,
                      )?.name ??
                      t("statusPages.serverEntry", { number: index + 1 }),
                  })}
                >
                  <TextInput
                    required
                    maxLength={100}
                    value={entry.name}
                    onChange={(event) =>
                      setEntries((current) =>
                        current.map((item) =>
                          item.key === entry.key
                            ? { ...item, name: event.target.value }
                            : item,
                        ),
                      )
                    }
                  />
                </FormField>
              </>
            )}
          </EditableList>
          <InlineMessage tone="warning">
            {t("statusPages.publicWarning")}
          </InlineMessage>
          {page && page.slug !== slug && (
            <InlineMessage tone="warning">
              {t("statusPages.urlWarning")}
            </InlineMessage>
          )}
        </Fieldset>
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {deleteError && (
          <InlineMessage role="alert">{deleteError}</InlineMessage>
        )}
      </form>
    </Drawer>
  );
}
