import { lazy, Suspense } from "react";
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
import { canPerformPlatform } from "./permissions";
import { ConnectionStatusProvider } from "./components/status/ConnectionStatus";

const PublicStatusPage = lazy(() =>
  import("./pages/PublicStatusPage").then((m) => ({
    default: m.PublicStatusPage,
  })),
);

export function App() {
  const { t } = useTranslation();
  const path = window.location.pathname;
  if (path === "/status" || path.startsWith("/status/")) {
    const match = /^\/status\/([a-z0-9][a-z0-9-]{1,62}[a-z0-9])\/?$/.exec(path);
    return (
      <Suspense
        fallback={
          <main className="public-status">
            <p>{t("common.loading")}</p>
          </main>
        }
      >
        <PublicStatusPage key={path} slug={match?.[1] ?? null} />
      </Suspense>
    );
  }
  return (
    <AuthenticationProvider>
      <AuthenticationGate />
    </AuthenticationProvider>
  );
}

function AuthenticationGate() {
  useTranslation();
  const auth = useAuthentication();
  return auth.session?.authenticated ? (
    <ConnectionStatusProvider key={auth.session.user_id}>
      <ManagementApp />
    </ConnectionStatusProvider>
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
  const allowed =
    canPerformPlatform(auth.session, "administration") || !route.adminOnly;

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
