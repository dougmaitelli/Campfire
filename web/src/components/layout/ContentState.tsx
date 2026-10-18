import type { ReactNode } from "react";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { Button } from "../forms/Button";
import "./content-state.css";

export function ContentState({
  loading = false,
  error,
  resource = "records",
  emptyMessage,
  emptyDescription,
  emptyAction,
  onRetry,
  compact = false,
}: {
  loading?: boolean;
  error?: string | null | undefined;
  resource?: string;
  emptyMessage?: ReactNode;
  emptyDescription?: ReactNode;
  emptyAction?: ReactNode;
  onRetry?: (() => void) | undefined;
  compact?: boolean;
}) {
  return (
    <div
      className={[
        "content-state",
        loading && "content-state-loading",
        compact && "content-state-compact",
      ]
        .filter(Boolean)
        .join(" ")}
      role={error && !loading ? "alert" : "status"}
      aria-busy={loading}
    >
      <span className="content-state-icon">
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
      {!loading &&
        (error ? (
          <>
            <p>{error}</p>
            {onRetry && (
              <Button type="button" onClick={onRetry}>
                Try again
              </Button>
            )}
          </>
        ) : (
          <>
            {emptyDescription && <p>{emptyDescription}</p>}
            {emptyAction}
          </>
        ))}
    </div>
  );
}
