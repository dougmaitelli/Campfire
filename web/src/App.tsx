import { DashboardLayout } from "./components/layout/DashboardLayout";
import { EmptyPage } from "./pages/EmptyPage";
import { SparksPage } from "./pages/SparksPage";
import { useCurrentPage } from "./routing/useCurrentPage";

export function App() {
  const page = useCurrentPage();

  return (
    <DashboardLayout page={page}>
      {page.id === "sparks" ? <SparksPage /> : <EmptyPage title={page.label} />}
    </DashboardLayout>
  );
}
