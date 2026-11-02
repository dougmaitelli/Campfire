import cronstrue from "cronstrue";
import i18n from "../i18n";

export function formatCronSchedule(expression: string, action: string): string {
  const normalized = expression.trim().replace(/\s+/g, " ");
  const fallback = () => i18n.t("scheduleNames.recurring", { action });
  // Match Campfire's five-field syntax; seconds and years are not supported.
  if (normalized.split(" ").length !== 5) return fallback();
  try {
    const description = cronstrue
      .toString(normalized, {
        use24HourTimeFormat: false,
        throwExceptionOnParseError: true,
      })
      .replace(
        /\b(\d{1,2}):(\d{2}) (AM|PM)\b/g,
        (_, hour: string, minute: string, period: string) =>
          `${Number(hour)}${minute === "00" ? "" : `:${minute}`}${period.toLowerCase()}`,
      );
    const time = description.match(/^At (.+)$/)?.[1];
    if (/^\d+ \d+ \* \* \*$/.test(normalized) && time)
      return i18n.t("scheduleNames.daily", { action, time });
    if (normalized === "0 * * * *")
      return i18n.t("scheduleNames.hourly", { action });
    return i18n.t("scheduleNames.description", {
      action,
      description: description.charAt(0).toLowerCase() + description.slice(1),
    });
  } catch {
    // Expressions are incomplete while typing; the server validates on preview.
    return fallback();
  }
}
