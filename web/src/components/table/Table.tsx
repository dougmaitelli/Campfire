import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import type { ComponentProps, ReactNode } from "react";
import { ContentState } from "../layout/ContentState";
import "./table.css";

export function Table({
  className,
  children,
  empty = false,
  loading = false,
  error,
  resource = i18n.t("table.records"),
  emptyMessage,
  emptyDescription,
  emptyAction,
  onRetry,
  ...props
}: ComponentProps<"table"> & {
  empty?: boolean;
  loading?: boolean;
  error?: string | null;
  resource?: string;
  emptyMessage?: string;
  emptyDescription?: string;
  emptyAction?: ReactNode;
  onRetry?: () => void;
}) {
  useTranslation();
  if (empty)
    return (
      <ContentState
        loading={loading}
        error={error}
        resource={resource}
        emptyMessage={emptyMessage}
        emptyDescription={emptyDescription}
        emptyAction={emptyAction}
        onRetry={onRetry}
      />
    );
  return (
    <div className="table-container">
      <table
        {...props}
        className={className ? `data-table ${className}` : "data-table"}
      >
        {children}
      </table>
    </div>
  );
}

export function TableRow({
  onOpen,
  className,
  children,
  ...props
}: ComponentProps<"tr"> & { onOpen?: (() => void) | undefined }) {
  useTranslation();
  return (
    <tr
      {...props}
      className={[onOpen ? "table-row-interactive" : "", className]
        .filter(Boolean)
        .join(" ")}
      onClick={(event) => {
        if (
          event.target instanceof Element &&
          event.target.closest(
            "a, button, input, select, textarea, summary, details, [role=button]",
          )
        )
          return;
        if (window.getSelection()?.toString()) return;
        onOpen?.();
      }}
    >
      {children}
    </tr>
  );
}
