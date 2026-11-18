import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { serverOptions, type ServerOptions } from "../api/access";
import { listServers, type Server } from "../api/servers";
import {
  cancelBackupImport,
  prepareStoredBackup,
  type BackupImport,
} from "../api/backupImports";
import { Drawer } from "../components/layout/Drawer";
import { InlineMessage } from "../components/status/InlineMessage";
import { ServerDrawer } from "./ServerDrawer";
import { Button } from "../components/forms/Button";

export function StoredBackupServerDrawer({
  archiveID,
  onClose,
  onSaved,
}: {
  archiveID: string;
  onClose: () => void;
  onSaved: (server: Server) => void;
}) {
  const { t } = useTranslation();
  const [prepared, setPrepared] = useState<{
    backup: BackupImport;
    options: ServerOptions;
    servers: Server[];
  } | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    let token: string | undefined;
    void Promise.all([
      prepareStoredBackup(archiveID, controller.signal).then((backup) => {
        token = backup.token;
        if (controller.signal.aborted) void cancelBackupImport(token);
        return backup;
      }),
      serverOptions(controller.signal),
      listServers(controller.signal),
    ])
      .then(([backup, options, servers]) => {
        if (!controller.signal.aborted)
          setPrepared({ backup, options, servers });
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : t("backupImports.failed"),
          );
      });
    return () => {
      controller.abort();
      if (token) void cancelBackupImport(token);
    };
  }, [archiveID, t]);
  if (prepared)
    return (
      <ServerDrawer
        server={null}
        backup={prepared.backup}
        sparks={prepared.options.sparks}
        embers={[prepared.backup.ember]}
        s3Configured={prepared.options.s3_available}
        servers={prepared.servers}
        onClose={onClose}
        onSaved={onSaved}
      />
    );
  return (
    <Drawer
      title={t("backupImports.create")}
      description={t("backupLibrary.prepareDescription")}
      onClose={onClose}
      busy={false}
      footerLeft={<Button onClick={onClose}>{t("common.cancel")}</Button>}
      footerRight={null}
    >
      {error ? (
        <InlineMessage role="alert">{error}</InlineMessage>
      ) : (
        <p role="status">{t("backupImports.validating")}</p>
      )}
    </Drawer>
  );
}
