import type { ComponentProps, ReactNode } from "react";
import { NavigationIcon } from "../navigation/NavigationIcon";
import "./table.css";

export function Table({
  className,
  children,
  empty = false,
  loading = false,
  error,
  resource = "records",
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
  if (empty)
    return (
      <div
        className={`table-state${loading ? " table-state-loading" : ""}`}
        role={error && !loading ? "alert" : "status"}
        aria-busy={loading}
      >
        <span className="table-state-icon">
          <NavigationIcon
            name={loading ? "loading" : error ? "warning" : "table"}
          />
        </span>
        <h2>
          {loading
            ? `Loading ${resource}…`
            : error
              ? `Unable to load ${resource}`
              : (emptyMessage ?? `No ${resource} yet`)}
        </h2>
        {error && !loading ? (
          <>
            <p>{error}</p>
            {onRetry && (
              <button type="button" className="button" onClick={onRetry}>
                Try again
              </button>
            )}
          </>
        ) : (
          !loading && (
            <>
              {emptyDescription && <p>{emptyDescription}</p>}
              {emptyAction}
            </>
          )
        )}
      </div>
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
