import { useTranslation } from "react-i18next";
import { CampfireMark } from "../navigation/CampfireMark";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { ArrowLink } from "../navigation/ArrowLink";
import "./overview-empty-state.css";

export function OverviewEmptyState({
  sparkCount,
  canManageSparks,
  canManageEmbers,
  canCreateServers,
}: {
  sparkCount: number | undefined;
  canManageSparks: boolean;
  canManageEmbers: boolean;
  canCreateServers: boolean;
}) {
  const { t } = useTranslation();
  const hasSpark = sparkCount !== undefined && sparkCount > 0;
  const canSetUp = canManageSparks || canManageEmbers;
  return (
    <section
      className="overview-empty-state"
      aria-labelledby="overview-setup-title"
    >
      <span className="overview-empty-mark">
        <CampfireMark />
      </span>
      <h2 id="overview-setup-title">
        {t(canSetUp ? "overviewEmptyState.title" : "serversPage.noServersYet")}
      </h2>
      <p>
        {t(
          canSetUp
            ? "overviewEmptyState.description"
            : "overviewEmptyState.askAdministrator",
        )}
      </p>
      {canSetUp && (
        <ol className="overview-setup-steps">
          <li>
            <span
              className={`overview-setup-marker${hasSpark ? " overview-setup-complete" : ""}`}
            >
              {hasSpark ? <NavigationIcon name="check" /> : "1"}
              {hasSpark && (
                <span className="visually-hidden">
                  {t("overviewEmptyState.complete")}
                </span>
              )}
            </span>
            <div>
              <h3>{t("overviewEmptyState.connectSpark")}</h3>
              <p>
                {t(
                  hasSpark
                    ? "overviewEmptyState.sparkReady"
                    : "overviewEmptyState.sparkDescription",
                )}
              </p>
            </div>
            {canManageSparks && (
              <ArrowLink href="#sparks">
                {t(
                  hasSpark
                    ? "overviewEmptyState.viewSparks"
                    : "overviewEmptyState.addSpark",
                )}
              </ArrowLink>
            )}
          </li>
          <li>
            <span className="overview-setup-marker">2</span>
            <div>
              <h3>{t("overviewEmptyState.importEmber")}</h3>
              <p>{t("overviewEmptyState.emberDescription")}</p>
            </div>
            {canManageEmbers && (
              <ArrowLink href="#embers">
                {t("overviewEmptyState.importEmber")}
              </ArrowLink>
            )}
          </li>
        </ol>
      )}
      {canCreateServers && (hasSpark || !canSetUp) && (
        <ArrowLink className="overview-setup-servers" href="#servers">
          {t("overviewEmptyState.goToServers")}
        </ArrowLink>
      )}
    </section>
  );
}
