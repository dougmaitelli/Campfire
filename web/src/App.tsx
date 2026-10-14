import { DashboardLayout } from "./components/layout/DashboardLayout";
import { EmptyPage } from "./pages/EmptyPage";
import { useCurrentPage } from "./routing/useCurrentPage";

export function App() {
  const page = useCurrentPage();

  return (
    <DashboardLayout page={page}>
      <EmptyPage title={page.label} />
    </DashboardLayout>
  );
}
