import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { Server } from "../../api/servers";
import { formatBytes, BYTES_PER_MEBIBYTE } from "../../utils/formatBytes";
import { GameArt } from "../media/GameArt";
import { NavigationIcon } from "../navigation/NavigationIcon";
import "./server-installation-card.css";

export function ServerInstallationCard({
  server,
  action,
}: {
  server: Server;
  action?: ReactNode;
}) {
  const { t, i18n } = useTranslation();
  return (
    <section
      className="server-install-card"
      aria-labelledby="server-install-title"
    >
      <span className="server-install-art" aria-hidden="true">
        <NavigationIcon name="servers" />
        <GameArt src={server.ember_art_url} />
      </span>
      <p className="server-install-ember">{server.ember_name}</p>
      <h2 id="server-install-title">
        {t("serverOperationsPage.readyToInstall")}
      </h2>
      <p className="server-install-description">
        {t(
          action
            ? "serverOperationsPage.installBeforeStarting"
            : "serverOperationsPage.askOperatorToInstall",
        )}
      </p>
      <dl className="server-install-allocation">
        <div>
          <dt>
            <NavigationIcon name="cpu" />
            <span className="visually-hidden">{t("common.cpu")}</span>
          </dt>
          <dd>
            {server.cpu_limit > 0
              ? t("serverOperationsPage.cpuAllocation", {
                  count: server.cpu_limit,
                  value: server.cpu_limit.toLocaleString(i18n.resolvedLanguage),
                })
              : t("common.unlimited")}
          </dd>
        </div>
        <div>
          <dt>
            <NavigationIcon name="memory" />
            <span className="visually-hidden">{t("common.memory")}</span>
          </dt>
          <dd>{formatBytes(server.memory_mb * BYTES_PER_MEBIBYTE)}</dd>
        </div>
        <div>
          <dt>
            <NavigationIcon name="disk" />
            <span className="visually-hidden">{t("common.disk")}</span>
          </dt>
          <dd>
            {server.disk_mb > 0
              ? formatBytes(server.disk_mb * BYTES_PER_MEBIBYTE)
              : t("common.unlimited")}
          </dd>
        </div>
      </dl>
      {action}
    </section>
  );
}
