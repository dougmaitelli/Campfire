import { DashboardLayout } from "./components/layout/DashboardLayout";
import { EmptyPage } from "./pages/EmptyPage";
import { SparksPage } from "./pages/SparksPage";
import { ServersPage } from "./pages/ServersPage";
import { EmbersPage } from "./pages/EmbersPage";
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

  return (
    <DashboardLayout page={page}>
      {page.id === "sparks" ? (
        <SparksPage />
      ) : page.id === "servers" ? (
        <ServersPage />
      ) : page.id === "embers" ? (
        <EmbersPage />
      ) : (
        <EmptyPage title={page.label} />
      )}
    </DashboardLayout>
  );
}
