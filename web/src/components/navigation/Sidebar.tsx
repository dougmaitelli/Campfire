import { useTranslation } from "react-i18next";
import { navigation } from "../../routing/navigation";
import type { NavigationItem } from "../../routing/navigation";
import { NavigationIcon } from "./NavigationIcon";
import { CampfireMark } from "./CampfireMark";
import { useAuthentication } from "../../hooks/useAuthentication";
import { AccountMenu } from "./AccountMenu";

export function Sidebar({ page }: { page: NavigationItem }) {
  const { t } = useTranslation();
  const auth = useAuthentication();
  return (
    <aside className="sidebar">
      <a
        className="brand"
        href="#dashboard"
        aria-label={t("sidebar.campfireDashboard")}
      >
        <span className="brand-mark">
          <CampfireMark />
        </span>
        <span>{t("sidebar.campfire")}</span>
      </a>
      <nav className="navigation" aria-label={t("sidebar.mainNavigation")}>
        {navigation
          .filter(
            (item) =>
              item.id !== "notifications" &&
              (auth.session?.admin === true ||
                item.id === "servers" ||
                item.id === "dashboard" ||
                item.id === "schedules"),
          )
          .map((item) => (
            <a
              key={item.id}
              href={`#${item.id}`}
              className="navigation-link"
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
