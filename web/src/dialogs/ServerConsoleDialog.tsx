import { useTranslation } from "react-i18next";
import { Button } from "../components/forms/Button";
import { lazy, Suspense, useState } from "react";
import type { Server } from "../api/servers";
import type { RuntimeStatus } from "../api/runtime";
import { Drawer } from "../components/layout/Drawer";
const ServerTerminal = lazy(() =>
  import("../components/servers/ServerTerminal").then((module) => ({
    default: module.ServerTerminal,
  })),
);

export function ServerConsoleDialog({
  server,
  onClose,
  embedded = false,
  status,
}: {
  server: Server;
  onClose: () => void;
  embedded?: boolean;
  status?: RuntimeStatus | undefined;
}) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  return (
    <Drawer
      embedded={embedded}
      title={t("serverConsoleDialog.console")}
      description={t("serverConsoleDialog.liveServerOutputAndCommands")}
      busy={busy}
      onClose={onClose}
      footerLeft={
        embedded ? null : (
          <Button disabled={busy} onClick={onClose}>
            {t("common.close")}
          </Button>
        )
      }
      footerRight={null}
    >
      <div className="drawer-content">
        <Suspense
          fallback={
            <p role="status">{t("serverConsoleDialog.loadingConsole")}</p>
          }
        >
          <ServerTerminal
            serverID={server.id}
            streamKey={`${status?.operation_id ?? ""}:${status?.busy ?? false}`}
            streamMode={
              !status || status.state === "unknown"
                ? "waiting"
                : status.busy ||
                    ["running", "restarting", "starting"].includes(status.state)
                  ? "live"
                  : "snapshot"
            }
            onBusyChange={setBusy}
          />
        </Suspense>
      </div>
    </Drawer>
  );
}
