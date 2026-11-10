import { useTranslation } from "react-i18next";
import { StatusPill, type StatusTone } from "../status/StatusPill";
import type { ActivityEntry } from "../../api/serverActivity";
import i18n from "../../i18n";
import { ServerAction } from "../../contracts/domains";
import { operationLabel } from "../servers/operationMessage";

export function activityTitle(entry: ActivityEntry): string {
  if (entry.kind === "job")
    return i18n.t(
      entry.title === "copy" ? "serverCopy.copy" : "serversPage.transfer",
    );
  if (entry.kind === "operation")
    return operationLabel(
      Object.values(ServerAction).find((action) => action === entry.title),
    );
  return entry.title;
}

export function ActivityStatus({ state }: { state: string }) {
  const { t } = useTranslation();
  if (!state) return <span>—</span>;
  const tone: StatusTone = [
    "failed",
    "dispatch_failed",
    "interrupted",
  ].includes(state)
    ? "negative"
    : ["completed", "delivered", "succeeded", "recovered"].includes(state)
      ? "positive"
      : ["running", "dispatching", "pending", "cancelling"].includes(state)
        ? "pending"
        : ["active", "raised", "uncertain"].includes(state)
          ? "warning"
          : "neutral";
  return (
    <StatusPill tone={tone}>
      {t(`serverActivity.states.${state}`, { defaultValue: state })}
    </StatusPill>
  );
}
