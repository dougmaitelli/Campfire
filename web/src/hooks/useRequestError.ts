import { createContext, useContext, useEffect, useId } from "react";
import { ConnectionError } from "../api/client";

export const RequestErrorContext = createContext<{
  disconnected: boolean;
  report: (id: string, failed: boolean) => void;
} | null>(null);

// Each mounted reader owns its report. A successful sibling request must not
// clear another reader's failure, and unmounting must not leave a stale report.
export function useRequestError(cause: Error | string | null | undefined) {
  const context = useContext(RequestErrorContext);
  const report = context?.report;
  const id = useId();
  const connectionError = cause instanceof ConnectionError && !!report;
  useEffect(() => {
    if (!connectionError) return;
    report(id, true);
    return () => report(id, false);
  }, [connectionError, report, id]);
  return {
    error: connectionError
      ? null
      : cause instanceof Error
        ? cause.message
        : (cause ?? null),
    connectionError,
  };
}
