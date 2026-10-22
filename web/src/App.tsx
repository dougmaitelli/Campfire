import { Suspense } from "react";
import { useTranslation } from "react-i18next";
import { ResourceInspectionProvider } from "./routing/ResourceInspection";
import { routeRegistry } from "./routing/routeRegistry";
import { DashboardLayout } from "./components/layout/DashboardLayout";
import { EmptyPage } from "./pages/EmptyPage";
import { ContentState } from "./components/layout/ContentState";
import { NotificationsProvider } from "./hooks/useNotifications";
import { useCurrentPage } from "./routing/useCurrentPage";
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
  const auth = useAuthentication();
  const route = routeRegistry[page.id];
  const Page = route.Page;
  const allowed = auth.session?.admin === true || !route.adminOnly;

  return (
    <NotificationsProvider key={auth.session?.user_id}>
      <ResourceInspectionProvider key={page.id}>
        <DashboardLayout page={page}>
          {!allowed ? (
            <EmptyPage title={t("app.youDoNotHaveAccessToThisPage")} />
          ) : (
            <Suspense fallback={<ContentState loading resource={page.label} />}>
              <Page />
            </Suspense>
          )}
        </DashboardLayout>
      </ResourceInspectionProvider>
    </NotificationsProvider>
  );
}
