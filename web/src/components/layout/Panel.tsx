import { useId, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import "./panel.css";

interface PanelProps {
  title: ReactNode;
  description?: string | undefined;
  headingID?: string;
  headingActions?: ReactNode;
  leading?: ReactNode;
  children: ReactNode;
  footerLeft?: ReactNode;
  footerRight?: ReactNode;
}

export function Panel({
  title,
  description,
  headingID,
  headingActions,
  leading,
  children,
  footerLeft,
  footerRight,
}: PanelProps) {
  const generatedID = useId();
  const id = headingID ?? generatedID;
  const { t } = useTranslation();
  return (
    <section className="panel" aria-labelledby={id}>
      {leading && <div className="panel-leading">{leading}</div>}
      <header className="panel-heading">
        <div>
          <h2 id={id} className="panel-title">
            {title}
          </h2>
          {description && <p id={`${id}-description`}>{description}</p>}
        </div>
        {headingActions != null && (
          <div
            className="panel-heading-actions"
            role="group"
            aria-label={t("common.actions")}
          >
            {headingActions}
          </div>
        )}
      </header>
      <div className="panel-content-area">{children}</div>
      {(footerLeft != null || footerRight != null) && (
        <footer className="panel-footer">
          <div className="panel-footer-left">{footerLeft}</div>
          <div className="panel-footer-right">{footerRight}</div>
        </footer>
      )}
    </section>
  );
}
