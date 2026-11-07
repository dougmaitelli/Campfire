import { useTranslation } from "react-i18next";
import type { Server } from "../../api/servers";

export function ServerConfigurationNotices({
  server,
  restartRequired,
}: {
  server: Server | undefined;
  restartRequired: boolean | undefined;
}) {
  const { t } = useTranslation();
  return (
    <>
      {server?.ember_update_available && (
        <span className="row-description server-configuration-notice">
          {t("emberUpdate.available")}
        </span>
      )}
      {restartRequired && (
        <span className="row-description server-configuration-notice">
          {t("emberUpdate.restartRequired")}
        </span>
      )}
    </>
  );
}
