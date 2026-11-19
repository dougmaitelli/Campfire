import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import "./table-interactions.css";

export function TableSelectionHelp({
  children,
  marquee = false,
}: {
  children?: ReactNode;
  marquee?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <details className="table-selection-help">
      <summary>{t("table.selectionShortcuts")}</summary>
      <p>{t("table.selectionHelp")}</p>
      {marquee && <p>{t("table.marqueeHelp")}</p>}
      {children}
    </details>
  );
}
