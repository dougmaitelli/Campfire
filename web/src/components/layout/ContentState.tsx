import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import { useContext, type ComponentProps, type ReactNode } from "react";
import { RequestErrorContext } from "../../hooks/useRequestError";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { Button } from "../forms/Button";
import "./content-state.css";

export function ContentState({
  loading = false,
  error,
  resource = i18n.t("contentState.records"),
  emptyMessage,
  emptyDescription,
  emptyAction,
  emptyIcon = "table",
  onRetry,
  compact = false,
}: {
  loading?: boolean;
  error?: string | null | undefined;
  resource?: string;
  emptyMessage?: ReactNode;
  emptyDescription?: ReactNode;
  emptyAction?: ReactNode;
  emptyIcon?: ComponentProps<typeof NavigationIcon>["name"] | undefined;
  onRetry?: (() => void) | undefined;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const connection = useContext(RequestErrorContext);
  const unavailable = connection?.disconnected && !loading && !error;
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
          name={loading ? "loading" : error ? "warning" : emptyIcon}
        />
      </span>
      <h2>
        {loading
          ? t("contentState.loading", { resource: resource })
          : error
            ? t("contentState.unableToLoad", { resource: resource })
            : unavailable
              ? t("connection.dataUnavailable")
              : (emptyMessage ??
                t("contentState.noYet", { resource: resource }))}
      </h2>
      {!loading &&
        (error ? (
          <>
            <p>{error}</p>
            {onRetry && (
              <Button type="button" onClick={onRetry}>
                {t("common.tryAgain")}
              </Button>
            )}
          </>
        ) : !unavailable ? (
          <>
            {emptyDescription && <p>{emptyDescription}</p>}
            {emptyAction}
          </>
        ) : null)}
    </div>
  );
}
