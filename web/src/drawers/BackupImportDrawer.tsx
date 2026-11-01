import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Drawer } from "../components/layout/Drawer";
import { FormField } from "../components/forms/FormField";
import { TextInput } from "../components/forms/TextInput";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { ProgressBar } from "../components/status/ProgressBar";
import { DetailsList, DetailItem } from "../components/layout/DetailsList";
import { formatBytes } from "../utils/formatBytes";
import { BackupMode } from "../api/backups";
import {
  cancelBackupImport,
  publishBackupImport,
  uploadPortableBackup,
  type BackupImport,
} from "../api/backupImports";

export function BackupImportDrawer({
  serverID,
  onClose,
  onReady,
}: {
  serverID?: string;
  onClose: () => void;
  onReady: (backup: BackupImport) => void;
}) {
  const { t } = useTranslation();
  const [backup, setBackup] = useState<BackupImport | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const [bytes, setBytes] = useState<number | null>(0);
  const [busy, setBusy] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const controller = useRef<AbortController | null>(null);
  const retained = useRef(false);
  const token = useRef<string | null>(null);
  useEffect(
    () => () => {
      controller.current?.abort();
      if (token.current && !retained.current)
        void cancelBackupImport(token.current);
    },
    [],
  );
  async function choose(selected: File) {
    if (busy) return;
    if (token.current) await cancelBackupImport(token.current);
    token.current = null;
    setBackup(null);
    setFile(selected);
    setBytes(0);
    setError(null);
    setBusy(true);
    const active = new AbortController();
    controller.current = active;
    try {
      const next = await uploadPortableBackup(
        selected,
        serverID,
        active.signal,
        setBytes,
      );
      token.current = next.token;
      setBackup(next);
    } catch (cause) {
      if (!active.signal.aborted)
        setError(
          cause instanceof Error ? cause.message : t("backupImports.failed"),
        );
    } finally {
      setBusy(false);
    }
  }
  async function finish() {
    if (!backup || busy) return;
    if (!serverID) {
      retained.current = true;
      onReady(backup);
      return;
    }
    setBusy(true);
    setPublishing(true);
    setError(null);
    try {
      await publishBackupImport(serverID, backup.token);
      token.current = null;
      onReady(backup);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("backupImports.failed"),
      );
    } finally {
      setBusy(false);
      setPublishing(false);
    }
  }
  return (
    <Drawer
      title={t(serverID ? "backupImports.upload" : "backupImports.create")}
      description={t("backupImports.description")}
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button
          disabled={publishing}
          onClick={() => {
            controller.current?.abort();
            onClose();
          }}
        >
          {t("common.cancel")}
        </Button>
      }
      footerRight={
        <Button
          variant="primary"
          disabled={!backup || busy}
          onClick={() => {
            void finish();
          }}
        >
          {t(serverID ? "backupImports.upload" : "backupImports.continue")}
        </Button>
      }
    >
      <div className="panel-content">
        <FormField label={t("backupImports.archive")}>
          <TextInput
            type="file"
            accept=".tar.gz,application/gzip"
            disabled={busy}
            onChange={(event) => {
              const selected = event.target.files?.[0];
              if (selected) void choose(selected);
            }}
          />
        </FormField>
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {busy && (
          <section aria-label={t("backupImports.progress")}>
            <p role="status">
              {t(
                publishing
                  ? "backupImports.publishing"
                  : bytes === null
                    ? "backupImports.validating"
                    : "backupImports.uploading",
              )}
            </p>
            <ProgressBar
              max={file?.size ?? 1}
              value={publishing || bytes === null ? undefined : bytes}
              aria-label={t("backupImports.progress")}
            />
            {file && bytes !== null && !publishing && (
              <p>
                {formatBytes(bytes)} / {formatBytes(file.size)}
              </p>
            )}
          </section>
        )}
        {backup && !busy && (
          <>
            <DetailsList>
              <DetailItem label={t("common.name")}>
                {backup.configuration.name}
              </DetailItem>
              <DetailItem label={t("common.ember")}>
                {backup.ember.name}
              </DetailItem>
              <DetailItem label={t("backupModes.label")}>
                {t(
                  backup.mode === BackupMode.Savedata
                    ? "backupModes.savedata"
                    : "backupModes.full",
                )}
              </DetailItem>
            </DetailsList>
            <p>
              {t(
                serverID
                  ? "backupImports.uploadNotice"
                  : backup.mode === BackupMode.Savedata
                    ? "backupImports.savedataNotice"
                    : "backupImports.fullNotice",
              )}
            </p>
            {!serverID && backup.import_ember && (
              <InlineMessage tone="warning">
                {t("backupImports.importEmber")}
              </InlineMessage>
            )}
          </>
        )}
      </div>
    </Drawer>
  );
}
