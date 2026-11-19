import { useTranslation } from "react-i18next";
import { StatusPill, type StatusTone } from "../status/StatusPill";
import type { ActivityEntry } from "../../api/serverActivity";
import i18n from "../../i18n";
import { ServerAction, ActivityEntryKind } from "../../contracts/domains";
import { isEnumValue } from "../../utils/validation";
import { operationLabel } from "../servers/operationMessage";

export function activityTitle(entry: ActivityEntry): string {
  if (entry.kind === ActivityEntryKind.Job)
    return i18n.t(
      entry.title === "copy" ? "serverCopy.copy" : "serversPage.transfer",
    );
  if (entry.kind === ActivityEntryKind.Operation)
    return operationLabel(
      isEnumValue(ServerAction, entry.title) ? entry.title : undefined,
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
