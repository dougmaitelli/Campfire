import { useTranslation } from "react-i18next";
import { serverNeedsEmberReview, type Server } from "../../api/servers";

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
      {serverNeedsEmberReview(server) && (
        <span className="row-description server-configuration-notice">
          {t("emberUpdate.available")}
        </span>
      )}
      {server?.deployment_sync_required && (
        <span className="row-description server-configuration-notice">
          {t("emberUpdate.syncRequired")}
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
