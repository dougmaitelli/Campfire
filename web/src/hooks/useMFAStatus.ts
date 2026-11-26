import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { readMFA, type MFAStatus } from "../api/mfa";

export function useMFAStatus() {
  const { t } = useTranslation();
  const [status, setStatus] = useState<MFAStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  function refresh() {
    setStatus(null);
    setError(null);
    setLoading(true);
    setRevision((value) => value + 1);
  }
  useEffect(() => {
    const controller = new AbortController();
    void readMFA(controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) {
          setStatus(value);
          setError(null);
          setLoading(false);
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) {
          setStatus(null);
          setError(cause instanceof Error ? cause.message : t("mfa.failed"));
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [revision, t]);
  return { status, loading, error, refresh };
}

export type MFAState = ReturnType<typeof useMFAStatus>;
