import type { ReactNode } from "react";
import "./section-heading.css";

export function SectionHeading({
  title,
  icon,
  actions,
}: {
  title: ReactNode;
  icon?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div className="section-heading">
      <h2>
        {icon}
        {title}
      </h2>
      {actions}
    </div>
  );
}
