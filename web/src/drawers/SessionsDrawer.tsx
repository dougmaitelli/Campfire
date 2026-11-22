import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import i18n from "../i18n";
import {
  listSessions,
  revokeSession,
  type LoginSession,
} from "../api/sessions";
import { Drawer } from "../components/layout/Drawer";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { Table } from "../components/table/Table";
import { useConfirmation } from "../hooks/useConfirmation";
import { useAuthentication } from "../hooks/useAuthentication";

function deviceName(agent: string): string {
  const browser = /(Edg|EdgiOS|EdgA)\//.test(agent)
    ? "Edge"
    : /(Firefox|FxiOS)\//.test(agent)
      ? "Firefox"
      : /(Chrome|CriOS)\//.test(agent)
        ? "Chrome"
        : /Safari\//.test(agent)
          ? "Safari"
          : "";
  const device = /iPhone/.test(agent)
    ? "iPhone"
    : /iPad/.test(agent)
      ? "iPad"
      : /Android/.test(agent)
        ? "Android"
        : /Windows/.test(agent)
          ? "Windows"
          : /Macintosh/.test(agent)
            ? "macOS"
            : /Linux/.test(agent)
              ? "Linux"
              : "";
  return (
    [browser, device].filter(Boolean).join(" · ") ||
    i18n.t("sessions.unknownDevice")
  );
}

export function SessionsDrawer({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const auth = useAuthentication();
  const confirmation = useConfirmation();
  const [sessions, setSessions] = useState<LoginSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    void listSessions(controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setSessions(result);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : i18n.t("sessions.loadFailed"),
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, []);

  async function revoke(session?: LoginSession) {
    if (pending.current) return;
    pending.current = true;
    try {
      if (
        !(await confirmation.confirm({
          title: session ? t("sessions.revoke") : t("sessions.revokeOthers"),
          description: session?.current
            ? t("sessions.confirmCurrent")
            : session
              ? t("sessions.confirmOne", {
                  device: deviceName(session.user_agent),
                })
              : t("sessions.confirmOthers"),
          confirmLabel: t("accountMenu.signOut"),
          danger: true,
        }))
      )
        return;
      setBusy(true);
      setError(null);
      await revokeSession(session?.id);
      if (session?.current) {
        onClose();
        await auth.reload();
      } else {
        setSessions((current) =>
          current.filter((item) =>
            session ? item.id !== session.id : item.current,
          ),
        );
      }
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("sessions.updateFailed"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  function timestamp(value: number) {
    return value ? (
      <time dateTime={new Date(value * 1000).toISOString()}>
        {new Date(value * 1000).toLocaleString()}
      </time>
    ) : (
      t("sessions.unknownTime")
    );
  }

  return (
    <Drawer
      title={t("sessions.title")}
      description={t("sessions.description")}
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button disabled={busy} onClick={onClose}>
          {t("common.close")}
        </Button>
      }
      footerRight={
        <Button
          variant="danger"
          disabled={
            loading || busy || !sessions.some((session) => !session.current)
          }
          onClick={() => void revoke()}
        >
          {t("sessions.revokeOthers")}
        </Button>
      }
    >
      {confirmation.dialog}
      <div className="panel-content form-stack">
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        <Table
          aria-label={t("sessions.title")}
          empty={!sessions.length}
          loading={loading}
          resource={t("sessions.title")}
          emptyMessage={t("sessions.empty")}
        >
          <thead>
            <tr>
              <th scope="col">{t("sessions.device")}</th>
              <th scope="col" className="table-date">
                {t("sessions.signedIn")}
              </th>
              <th scope="col" className="table-date">
                {t("sessions.lastActive")}
              </th>
              <th scope="col" className="table-fit">
                <span className="visually-hidden">{t("common.actions")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((session) => (
              <tr key={session.id}>
                <td>
                  <span title={session.user_agent || undefined}>
                    {deviceName(session.user_agent)}
                  </span>
                  {session.current && (
                    <strong className="row-description">
                      {t("sessions.current")}
                    </strong>
                  )}
                </td>
                <td className="table-date">{timestamp(session.created_at)}</td>
                <td className="table-date">
                  {timestamp(session.last_seen_at)}
                </td>
                <td className="table-fit">
                  <Button
                    variant="danger"
                    disabled={busy}
                    aria-label={t("sessions.signOutDevice", {
                      device: session.current
                        ? t("sessions.current")
                        : deviceName(session.user_agent),
                    })}
                    onClick={() => void revoke(session)}
                  >
                    {t("accountMenu.signOut")}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      </div>
    </Drawer>
  );
}
