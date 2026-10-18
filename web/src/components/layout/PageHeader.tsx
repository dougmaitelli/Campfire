import type { ReactNode } from "react";
import "./page-header.css";

interface PageHeaderProps {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  leading?: ReactNode;
  badge?: ReactNode;
}

export function PageHeader({
  title,
  description,
  actions,
  leading,
  badge,
}: PageHeaderProps) {
  return (
    <header className="page-header">
      <div className="page-header-copy">
        {leading}
        <div className="page-header-title">
          <h1 id="page-title">{title}</h1>
          {badge}
        </div>
        {description && <p className="page-description">{description}</p>}
      </div>
      {actions && <div className="page-actions">{actions}</div>}
    </header>
  );
}
