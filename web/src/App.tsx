import { DashboardLayout } from "./components/layout/DashboardLayout";
import { EmptyPage } from "./pages/EmptyPage";
import { SparksPage } from "./pages/SparksPage";
import { ServersPage } from "./pages/ServersPage";
import { EmbersPage } from "./pages/EmbersPage";
import { UsersPage } from "./pages/UsersPage";
import { RolesPage } from "./pages/RolesPage";
import { SchedulesPage } from "./pages/SchedulesPage";
import { AuditPage } from "./pages/AuditPage";
import { DashboardPage } from "./pages/DashboardPage";
import { NotificationsPage } from "./pages/NotificationsPage";
import { NotificationsProvider } from "./hooks/useNotifications";
import { useCurrentPage } from "./routing/useCurrentPage";
import { AuthenticationPage } from "./pages/AuthenticationPage";
import {
  AuthenticationProvider,
  useAuthentication,
} from "./hooks/useAuthentication";

export function App() {
  return (
    <AuthenticationProvider>
      <AuthenticatedApp />
    </AuthenticationProvider>
  );
}

function AuthenticatedApp() {
  const auth = useAuthentication();
  return auth.session?.authenticated ? (
    <ManagementApp />
  ) : (
    <AuthenticationPage />
  );
}

function ManagementApp() {
  const page = useCurrentPage();
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
          <EmptyPage title="You do not have access to this page" />
        ) : page.id === "dashboard" ? (
          <DashboardPage />
        ) : page.id === "schedules" ? (
          <SchedulesPage />
        ) : page.id === "notifications" ? (
          <NotificationsPage />
        ) : page.id === "audit" ? (
          <AuditPage />
        ) : page.id === "users" ? (
          <UsersPage />
        ) : page.id === "roles" ? (
          <RolesPage />
        ) : page.id === "sparks" ? (
          <SparksPage />
        ) : page.id === "servers" ? (
          <ServersPage />
        ) : (
          <EmbersPage />
        )}
      </DashboardLayout>
    </NotificationsProvider>
  );
}
