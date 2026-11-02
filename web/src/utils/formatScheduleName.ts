import { ScheduleAction } from "../contracts/domains";
import { ResourceLimits } from "../config/policies";
import i18n from "../i18n";
import { formatCronSchedule } from "./formatCronSchedule";

interface ScheduleNameInput {
  serverName: string;
  action: ScheduleAction | "";
  timing:
    | { once: true; runAt: string }
    | { once: false; expression: string; timeZone: string };
}

const nameSeparator = " - ";

export function scheduleActionLabel(action: ScheduleAction): string {
  if (action === ScheduleAction.Command)
    return i18n.t("scheduleDrawer.consoleCommand");
  if (action === ScheduleAction.Backup)
    return i18n.t("operationMessage.backup");
  return i18n.t(`common.${action}`);
}

export function formatScheduleName({
  serverName,
  action,
  timing,
}: ScheduleNameInput): string {
  if (!serverName || !action) return "";
  const label = scheduleActionLabel(action);
  let description: string;
  if (timing.once) {
    const date = new Date(timing.runAt);
    const when = Number.isFinite(date.getTime())
      ? `${new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" }).format(date)} (${Intl.DateTimeFormat().resolvedOptions().timeZone})`
      : i18n.t("scheduleDrawer.runOnce");
    description = `${label}${nameSeparator}${when}`;
  } else {
    description = formatCronSchedule(timing.expression, label);
    if (
      timing.timeZone &&
      timing.timeZone !== Intl.DateTimeFormat().resolvedOptions().timeZone
    )
      description += ` (${timing.timeZone})`;
  }
  const server = Array.from(serverName);
  const details = Array.from(description);
  const available = ResourceLimits.NameLength - nameSeparator.length;
  const serverLength = Math.min(
    server.length,
    Math.max(Math.floor(available / 2), available - details.length),
  );
  return [
    shorten(server, serverLength),
    shorten(details, available - serverLength),
  ].join(nameSeparator);
}

function shorten(value: string[], length: number): string {
  return value.length > length
    ? `${value.slice(0, Math.max(0, length - 1)).join("")}…`
    : value.join("");
}
