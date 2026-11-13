import {
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import { RequestErrorContext } from "../../hooks/useRequestError";
import { InlineMessage } from "./InlineMessage";

export function ConnectionStatusProvider({
  children,
}: {
  children: ReactNode;
}) {
  const [failures, setFailures] = useState<Set<string>>(() => new Set());
  const report = useCallback((id: string, failed: boolean) => {
    setFailures((current) => {
      if (current.has(id) === failed) return current;
      const next = new Set(current);
      if (failed) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);
  const disconnected = failures.size > 0;
  const value = useMemo(
    () => ({ disconnected, report }),
    [disconnected, report],
  );
  return (
    <RequestErrorContext.Provider value={value}>
      {children}
    </RequestErrorContext.Provider>
  );
}

export function ConnectionNotice() {
  const { t } = useTranslation();
  const state = useContext(RequestErrorContext);
  return state?.disconnected ? (
    <InlineMessage tone="warning" role="alert">
      {t("connection.interrupted")}
    </InlineMessage>
  ) : null;
}
