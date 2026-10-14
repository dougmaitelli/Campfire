import { DashboardLayout } from "./components/layout/DashboardLayout";
import { EmptyPage } from "./pages/EmptyPage";
import { SparksPage } from "./pages/SparksPage";
import { ServersPage } from "./pages/ServersPage";
import { EmbersPage } from "./pages/EmbersPage";
import { useCurrentPage } from "./routing/useCurrentPage";

export function App() {
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
