import { lazy, Suspense, useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import type { Server } from "../../api/servers";
import { canPerformServer } from "../../permissions";
import { dismissLogAlert, listLogAlerts } from "../../api/logAlerts";
import { LogAction, LogPhase, LogSeverity } from "../../contracts/logAlerts";
import { useCollection } from "../../hooks/useCollection";
import { Button, ButtonLink } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import "./server-log-alerts.css";

const ServerSettingsDrawer = lazy(() =>
  import("./ServerSettingsDrawer").then((module) => ({
    default: module.ServerSettingsDrawer,
  })),
);

export function ServerLogAlerts({
  server,
  busy,
  monitorError,
  onServerSaved,
}: {
  server: Server;
  busy: boolean;
  monitorError?: string | undefined;
  onServerSaved: (server: Server) => void;
}) {
  const { t } = useTranslation();
  const list = useCallback(
    (signal: AbortSignal) => listLogAlerts(server.id, signal),
    [server.id],
  );
  const collection = useCollection(list, 3000);
  const [pending, setPending] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [settings, setSettings] = useState(false);
  const canDismiss = canPerformServer(server, "operate");
  const canReadConsole = canPerformServer(server, "readConsole");
  async function dismiss(id: string) {
    if (pending) return;
    setPending(id);
    setError(null);
    try {
      await dismissLogAlert(server.id, id);
      await collection.refresh();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("logAlerts.dismissFailed"),
      );
    } finally {
      setPending(null);
    }
  }
  const showAlerts =
    collection.items.length > 0 ||
    !!collection.error ||
    !!error ||
    !!monitorError;
  return (
    <>
      {showAlerts && (
        <section
          className="server-log-alerts"
          aria-label={t("logAlerts.needsAttention")}
        >
          <h2>{t("logAlerts.needsAttention")}</h2>
          {(error ?? collection.error) && (
            <InlineMessage>{error ?? collection.error}</InlineMessage>
          )}
          {monitorError && (
            <InlineMessage tone="warning">{monitorError}</InlineMessage>
          )}
          {collection.items.map((alert) => (
            <article
              key={alert.id}
              className={`server-log-alert server-log-alert-${alert.severity}`}
            >
              <div className="server-log-alert-heading">
                <strong>{alert.title}</strong>
                <span>
                  {t(
                    alert.severity === LogSeverity.Error
                      ? "logAlerts.error"
                      : "logAlerts.warning",
                  )}
                </span>
              </div>
              <p>{alert.message}</p>
              <p className="field-help">
                {t(
                  alert.phase === LogPhase.Installation
                    ? "emberEditor.installation"
                    : "emberEditor.runtime",
                )}{" "}
                ·{" "}
                <time
                  dateTime={new Date(alert.detected_at * 1000).toISOString()}
                >
                  {new Date(alert.detected_at * 1000).toLocaleString()}
                </time>
              </p>
              {canReadConsole && alert.line && (
                <details>
                  <summary>{t("logAlerts.matchedOutput")}</summary>
                  <pre>{alert.line}</pre>
                </details>
              )}
              <div className="server-log-alert-actions">
                {alert.action?.type === LogAction.OpenFile &&
                  alert.action.path &&
                  canPerformServer(server, "readFiles") && (
                    <ButtonLink
                      href={`#servers/${encodeURIComponent(server.id)}/files/${encodeURIComponent(alert.action.path)}`}
                    >
                      {t("logAlerts.openFile", { path: alert.action.path })}
                    </ButtonLink>
                  )}
                {alert.action?.type === LogAction.ServerSettings &&
                  canPerformServer(server, "configure") && (
                    <Button disabled={busy} onClick={() => setSettings(true)}>
                      {t("logAlerts.settingsAction")}
                    </Button>
                  )}
                {canReadConsole && (
                  <ButtonLink
                    href={`#servers/${encodeURIComponent(server.id)}/console`}
                  >
                    {t("logAlerts.viewConsole")}
                  </ButtonLink>
                )}
                {canDismiss && (
                  <Button
                    disabled={pending !== null}
                    onClick={() => void dismiss(alert.id)}
                  >
                    {t("logAlerts.dismiss")}
                  </Button>
                )}
              </div>
            </article>
          ))}
        </section>
      )}
      {settings && (
        <Suspense fallback={null}>
          <ServerSettingsDrawer
            server={server}
            restricted={busy}
            onClose={() => setSettings(false)}
            onSaved={onServerSaved}
          />
        </Suspense>
      )}
    </>
  );
}
