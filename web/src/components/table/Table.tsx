import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import type { ComponentProps } from "react";
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
  emptyIcon,
  onRetry,
  ...props
}: ComponentProps<"table"> &
  Pick<
    ComponentProps<typeof ContentState>,
    | "loading"
    | "error"
    | "resource"
    | "emptyMessage"
    | "emptyDescription"
    | "emptyAction"
    | "emptyIcon"
    | "onRetry"
  > & { empty?: boolean }) {
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
        emptyIcon={emptyIcon}
        onRetry={onRetry}
      />
    );
  return (
    <div className="table-container">
      <table
        {...props}
        role={props.role ?? "table"}
        className={className ? `data-table ${className}` : "data-table"}
      >
        {children}
      </table>
    </div>
  );
}

export function TableRow({
  onOpen,
  showCaret = true,
  className,
  children,
  ...props
}: ComponentProps<"tr"> & {
  onOpen?: (() => void) | undefined;
  showCaret?: boolean;
}) {
  useTranslation();
  return (
    <tr
      {...props}
      className={[
        onOpen ? "table-row-interactive" : "",
        onOpen && showCaret ? "table-row-caret" : "",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
      onClick={(event) => {
        if (
          event.target instanceof Element &&
          event.target.closest(
            "a, button, input, label, select, textarea, summary, details, [role=button]",
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
