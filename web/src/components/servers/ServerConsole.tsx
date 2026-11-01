import { RuntimeState } from "../../contracts/domains";
import { useTranslation } from "react-i18next";
import { lazy, Suspense, useState } from "react";
import type { Server } from "../../api/servers";
import type { RuntimeStatus } from "../../api/runtime";
import { useNavigationGuard } from "../../hooks/useNavigationGuard";
import { Permission } from "../../api/permissions";
const ServerTerminal = lazy(() =>
  import("./ServerTerminal").then((module) => ({
    default: module.ServerTerminal,
  })),
);

export function ServerConsole({
  server,
  status,
}: {
  server: Server;
  status?: RuntimeStatus | undefined;
}) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  useNavigationGuard(true, busy, false);
  return (
    <Suspense
      fallback={<p role="status">{t("serverConsole.loadingConsole")}</p>}
    >
      <ServerTerminal
        serverID={server.id}
        streamKey={`${status?.operation_id ?? ""}:${status?.busy ?? false}`}
        streamMode={
          !status || status.state === RuntimeState.Unknown
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
}
