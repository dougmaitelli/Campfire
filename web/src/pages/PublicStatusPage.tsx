import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { APIError, API_READ_TIMEOUT_MS } from "../api/client";
import {
  readPublicStatusPage,
  type PublicStatusPage as StatusSnapshot,
} from "../api/statusPages";
import { StatusPagePolling } from "../config/policies";
import { PublicServerStatus } from "../components/servers/PublicServerStatus";
import { CampfireMark } from "../components/navigation/CampfireMark";
import { Panel } from "../components/layout/Panel";
import { InlineMessage } from "../components/status/InlineMessage";
import { RefreshCw, Server } from "lucide-react";
import "./public-status.css";

export function PublicStatusPage({ slug }: { slug: string | null }) {
  const { t } = useTranslation();
  const [snapshot, setSnapshot] = useState<{
    slug: string;
    page: StatusSnapshot;
    requested: number;
  } | null>(null);
  const [failure, setFailure] = useState(false);
  const [missing, setMissing] = useState(false);
  const [now, setNow] = useState(() => performance.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(performance.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  useEffect(() => {
    if (!slug) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      const requested = performance.now();
      let nextPollSeconds: number = StatusPagePolling.RetrySeconds;
      try {
        const page = await readPublicStatusPage(
          slug!,
          AbortSignal.any([
            controller.signal,
            AbortSignal.timeout(API_READ_TIMEOUT_MS),
          ]),
        );
        if (controller.signal.aborted) return;
        nextPollSeconds = page.poll_seconds;
        setSnapshot({ slug: slug!, page, requested });
        setNow(performance.now());
        setFailure(false);
      } catch (cause) {
        if (controller.signal.aborted) return;
        if (cause instanceof APIError && cause.status === 404) {
          setSnapshot(null);
          setMissing(true);
          return;
        }
        setFailure(true);
      }
      timer = setTimeout(() => void load(), nextPollSeconds * 1000);
    }
    void load();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [slug]);
  const current = snapshot?.slug === slug ? snapshot : null;
  // Receipt timestamps and UI freshness constants stay unchanged. Anchor to
  // Campfire time and include the full request duration conservatively so a
  // slow response cannot renew old data. Wall-clock changes cannot renew it either.
  const serverNow = current
    ? current.page.collected_at + Math.max(0, now - current.requested) / 1000
    : 0;
  const unavailable = !slug || missing;
  return (
    <main className="public-status" aria-labelledby="public-status-title">
      <div className="public-status-masthead">
        <div className="public-status-brand">
          <CampfireMark />
          <span>{t("sidebar.campfire")}</span>
        </div>
        {!unavailable && (
          <p className="public-status-refresh">
            <RefreshCw size={14} aria-hidden="true" />
            {t("statusPages.updatesAutomatically")}
          </p>
        )}
      </div>
      <header className="public-status-heading">
        <p className="public-status-eyebrow">
          {t("statusPages.publicHeading")}
        </p>
        <h1 id="public-status-title">
          {unavailable
            ? t("statusPages.notFound")
            : (current?.page.title ?? t("statusPages.publicHeading"))}
        </h1>
      </header>
      {unavailable ? (
        <div className="public-status-empty public-status-surface">
          <Server size={32} aria-hidden="true" />
          <p>{t("statusPages.notFoundDetail")}</p>
        </div>
      ) : (
        <Panel
          className="public-status-surface"
          title={
            <>
              {t("common.servers")}
              {current && (
                <span className="public-status-count">
                  {current.page.entries.length}
                </span>
              )}
            </>
          }
        >
          {failure && (
            <div className="public-status-notice">
              <InlineMessage tone="warning" role="status">
                {t("statusPages.refreshFailed")}
              </InlineMessage>
            </div>
          )}
          {!current && !failure && (
            <div className="public-status-empty" role="status">
              <Server size={32} aria-hidden="true" />
              <p>{t("common.loading")}</p>
            </div>
          )}
          {current && (
            <>
              {current.page.entries.length === 0 ? (
                <div className="public-status-empty">
                  <Server size={32} aria-hidden="true" />
                  <p>{t("statusPages.noPublicServers")}</p>
                </div>
              ) : (
                <>
                  <ul className="public-status-list">
                    {current.page.entries.map((entry) => (
                      <PublicServerStatus
                        key={entry.id}
                        entry={entry}
                        now={serverNow}
                        failed={failure}
                      />
                    ))}
                  </ul>
                  {current.page.entries.some((entry) => entry.history) && (
                    <p className="public-status-history-key">
                      {t("statusPages.historyKey")}
                      <span className="history-online">
                        {t("statusPages.states.online")}
                      </span>
                      <span className="history-offline">
                        {t("statusPages.states.offline")}
                      </span>
                      <span className="history-unknown">
                        {t("statusPages.states.unknown")}
                      </span>
                    </p>
                  )}
                </>
              )}
            </>
          )}
        </Panel>
      )}
    </main>
  );
}
