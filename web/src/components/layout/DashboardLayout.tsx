import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import type { NavigationItem } from "../../routing/navigation";
import { Sidebar } from "../navigation/Sidebar";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { useNotifications } from "../../hooks/useNotifications";
import "./dashboard.css";

interface DashboardLayoutProps {
  page: NavigationItem;
  children: ReactNode;
}

export function DashboardLayout({ page, children }: DashboardLayoutProps) {
  const { t } = useTranslation();
  const inbox = useNotifications();
  return (
    <div className="dashboard-layout">
      <a className="skip-link" href="#main-content">
        {t("dashboardLayout.skipToContent")}
      </a>
      <Sidebar page={page} />
      <main id="main-content" className="main-content" tabIndex={-1}>
        <div className="app-toolbar">
          {import.meta.env.DEV &&
            import.meta.env.VITE_CAMPFIRE_MOCK === "true" && (
              <span className="mock-mode-notice" role="status">
                {t("dashboardLayout.mockModeSimulatedSparks")}
              </span>
            )}
          <a
            className="notification-link"
            href="#notifications"
            aria-label={
              inbox.unread === null
                ? t("common.notifications")
                : t("dashboardLayout.notificationsUnread", {
                    inboxUnread: inbox.unread,
                  })
            }
            aria-current={page.id === "notifications" ? "page" : undefined}
            title={t("common.notifications")}
          >
            <NavigationIcon name="notifications" />
            {inbox.unread !== null && inbox.unread > 0 && (
              <span className="notification-count" aria-hidden="true">
                {inbox.unread > 99 ? "99+" : inbox.unread}
              </span>
            )}
          </a>
        </div>
        {children}
      </main>
    </div>
  );
}
