import { ScheduleAction } from "../contracts/domains";
import { cronPresets } from "../config/cronPresets";
import { ResourceLimits } from "../config/policies";
import i18n from "../i18n";

interface ScheduleNameInput {
  serverName: string;
  action: ScheduleAction | "";
  timing:
    | { once: true; runAt: string }
    | { once: false; expression: string; timeZone: string };
}

const nameSeparator = " · ";

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
  let when: string;
  if (timing.once) {
    const date = new Date(timing.runAt);
    when = Number.isFinite(date.getTime())
      ? `${new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" }).format(date)} (${Intl.DateTimeFormat().resolvedOptions().timeZone})`
      : i18n.t("scheduleDrawer.runOnce");
  } else {
    const expression = timing.expression.trim().replace(/\s+/g, " ");
    const description =
      cronPresets.find((preset) => preset.value === expression)?.description ??
      expression;
    when = `${description || i18n.t("scheduleDrawer.recurring")} (${timing.timeZone})`;
  }
  const label = scheduleActionLabel(action);
  const server = Array.from(serverName);
  const time = Array.from(when);
  const available =
    ResourceLimits.NameLength -
    Array.from(label).length -
    nameSeparator.length * 2;
  const serverLength = Math.min(
    server.length,
    Math.max(Math.floor(available / 2), available - time.length),
  );
  return [
    shorten(server, serverLength),
    label,
    shorten(time, available - serverLength),
  ].join(nameSeparator);
}

function shorten(value: string[], length: number): string {
  return value.length > length
    ? `${value.slice(0, Math.max(0, length - 1)).join("")}…`
    : value.join("");
}
