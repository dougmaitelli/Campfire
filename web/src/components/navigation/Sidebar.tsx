import { useTranslation } from "react-i18next";
import { navigation } from "../../routing/navigation";
import { routeRegistry } from "../../routing/routeRegistry";
import type { NavigationItem } from "../../routing/navigation";
import { NavigationIcon } from "./NavigationIcon";
import { CampfireMark } from "./CampfireMark";
import { useAuthentication } from "../../hooks/useAuthentication";
import { canPerformPlatform } from "../../permissions";
import { AccountMenu } from "./AccountMenu";
import { Button } from "../forms/Button";
import "./sidebar.css";

export function Sidebar({
  page,
  onNavigate,
  onClose,
  className,
}: {
  page: NavigationItem;
  onNavigate?: () => void;
  onClose?: () => void;
  className?: string;
}) {
  const { t } = useTranslation();
  const auth = useAuthentication();
  return (
    <aside className={className ? `sidebar ${className}` : "sidebar"}>
      <div className="sidebar-header">
        <a
          className="brand"
          href="#dashboard"
          aria-label={t("sidebar.campfireDashboard")}
          onClick={onNavigate}
        >
          <span className="brand-mark">
            <CampfireMark />
          </span>
          <span>{t("sidebar.campfire")}</span>
        </a>
        {onClose && (
          <Button
            iconOnly
            className="navigation-close"
            aria-label={t("dashboardLayout.closeNavigation")}
            onClick={onClose}
          >
            <NavigationIcon name="close" />
          </Button>
        )}
      </div>
      <nav className="navigation" aria-label={t("sidebar.mainNavigation")}>
        {navigation
          .filter(
            (item) =>
              item.id !== "notifications" &&
              (canPerformPlatform(auth.session, "administration") ||
                !routeRegistry[item.id].adminOnly),
          )
          .map((item) => (
            <a
              key={item.id}
              href={`#${item.id}`}
              className="navigation-link"
              onClick={onNavigate}
              aria-current={page.id === item.id ? "page" : undefined}
            >
              <NavigationIcon name={item.icon} />
              <span>{item.label}</span>
            </a>
          ))}
      </nav>
      <AccountMenu />
    </aside>
  );
}
