import { useTranslation } from "react-i18next";
import { Button } from "../components/forms/Button";
import { lazy, Suspense, useState } from "react";
import type { Server } from "../api/servers";
import type { RuntimeStatus } from "../api/runtime";
import { Drawer } from "../components/layout/Drawer";
import { useNavigationGuard } from "../hooks/useNavigationGuard";
import { Permission } from "../api/permissions";
const ServerTerminal = lazy(() =>
  import("../components/servers/ServerTerminal").then((module) => ({
    default: module.ServerTerminal,
  })),
);

export function ServerConsoleDrawer({
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
  useNavigationGuard(embedded, busy, false);
  const content = (
    <Suspense
      fallback={<p role="status">{t("serverConsoleDrawer.loadingConsole")}</p>}
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
        canWrite={server.permissions.includes(Permission.ServerConsoleWrite)}
      />
    </Suspense>
  );
  if (embedded) return content;
  return (
    <Drawer
      title={t("serverConsoleDrawer.console")}
      description={t("serverConsoleDrawer.liveServerOutputAndCommands")}
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button disabled={busy} onClick={onClose}>
          {t("common.close")}
        </Button>
      }
      footerRight={null}
    >
      <div className="drawer-content">{content}</div>
    </Drawer>
  );
}
