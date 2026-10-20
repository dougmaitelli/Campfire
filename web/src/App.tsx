import { useTranslation } from "react-i18next";
import { DashboardLayout } from "./components/layout/DashboardLayout";
import { EmptyPage } from "./pages/EmptyPage";
import { SparksPage } from "./pages/SparksPage";
import { ServersPage } from "./pages/ServersPage";
import { ServerOperationsPage } from "./pages/ServerOperationsPage";
import { EmbersPage } from "./pages/EmbersPage";
import { UsersPage } from "./pages/UsersPage";
import { RolesPage } from "./pages/RolesPage";
import { SchedulesPage } from "./pages/SchedulesPage";
import { AuditPage } from "./pages/AuditPage";
import { BackupsPage } from "./pages/BackupsPage";
import { DashboardPage } from "./pages/DashboardPage";
import { NotificationsPage } from "./pages/NotificationsPage";
import { NotificationsProvider } from "./hooks/useNotifications";
import { useCurrentPage, useCurrentRoute } from "./routing/useCurrentPage";
import { AuthenticationPage } from "./pages/AuthenticationPage";
import {
  AuthenticationProvider,
  useAuthentication,
} from "./hooks/useAuthentication";

export function App() {
  useTranslation();
  return (
    <AuthenticationProvider>
      <AuthenticatedApp />
    </AuthenticationProvider>
  );
}

function AuthenticatedApp() {
  useTranslation();
  const auth = useAuthentication();
  return auth.session?.authenticated ? (
    <ManagementApp />
  ) : (
    <AuthenticationPage />
  );
}

function ManagementApp() {
  const { t } = useTranslation();
  const page = useCurrentPage();
  const route = useCurrentRoute().split("/");
  let serverID = "";
  try {
    serverID = decodeURIComponent(route[1] ?? "");
  } catch {
    /* Invalid routes return to the server list. */
  }
  const auth = useAuthentication();
  const allowed =
    auth.session?.admin === true ||
    page.id === "servers" ||
    page.id === "dashboard" ||
    page.id === "schedules" ||
    page.id === "notifications";

  return (
    <NotificationsProvider key={auth.session?.user_id}>
      <DashboardLayout page={page}>
        {!allowed ? (
          <EmptyPage title={t("app.youDoNotHaveAccessToThisPage")} />
        ) : page.id === "dashboard" ? (
          <DashboardPage />
        ) : page.id === "schedules" ? (
          <SchedulesPage />
        ) : page.id === "notifications" ? (
          <NotificationsPage />
        ) : page.id === "audit" ? (
          <AuditPage />
        ) : page.id === "backups" ? (
          <BackupsPage />
        ) : page.id === "users" ? (
          <UsersPage />
        ) : page.id === "roles" ? (
          <RolesPage />
        ) : page.id === "sparks" ? (
          <SparksPage />
        ) : page.id === "servers" ? (
          serverID ? (
            <ServerOperationsPage
              key={serverID}
              serverID={serverID}
              tab={route[2] ?? "console"}
            />
          ) : (
            <ServersPage />
          )
        ) : (
          <EmbersPage />
        )}
      </DashboardLayout>
    </NotificationsProvider>
  );
}
