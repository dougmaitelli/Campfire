import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { serverOptions, type ServerOptions } from "../../api/access";
import type { Server } from "../../api/servers";
import { ServerDrawer } from "../../drawers/ServerDrawer";
import { Drawer } from "../layout/Drawer";
import { Button } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";

export function ServerSettingsDrawer({
  server,
  restricted,
  onClose,
  onSaved,
}: {
  server: Server;
  restricted: boolean;
  onClose: () => void;
  onSaved: (server: Server) => void;
}) {
  const { t } = useTranslation();
  const [options, setOptions] = useState<ServerOptions | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    void serverOptions(controller.signal, server.id)
      .then((value) => {
        if (!controller.signal.aborted) setOptions(value);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : t("logAlerts.settingsFailed"),
          );
      });
    return () => controller.abort();
  }, [server.id, t]);
  if (!options)
    return (
      <Drawer
        title={t("logAlerts.settingsAction")}
        busy={false}
        onClose={onClose}
        footerLeft={<Button onClick={onClose}>{t("common.cancel")}</Button>}
        footerRight={null}
      >
        <div className="panel-content">
          {error ? (
            <InlineMessage>{error}</InlineMessage>
          ) : (
            <p role="status">{t("common.loading")}</p>
          )}
        </div>
      </Drawer>
    );
  return (
    <ServerDrawer
      server={server}
      runtimeRestricted={restricted}
      s3Configured={options.s3_available}
      sparks={options.sparks}
      embers={options.embers}
      servers={[server]}
      onClose={onClose}
      onSaved={onSaved}
    />
  );
}
