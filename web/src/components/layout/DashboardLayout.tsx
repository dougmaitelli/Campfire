import type { ReactNode } from "react";
import type { NavigationItem } from "../../routing/navigation";
import { Sidebar } from "../navigation/Sidebar";
import "./dashboard.css";

interface DashboardLayoutProps {
  page: NavigationItem;
  children: ReactNode;
}

export function DashboardLayout({ page, children }: DashboardLayoutProps) {
  return (
    <div className="dashboard-layout">
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <Sidebar page={page} />
      <main id="main-content" className="main-content" tabIndex={-1}>
        {children}
      </main>
    </div>
  );
}
