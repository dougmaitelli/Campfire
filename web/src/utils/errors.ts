import i18n from "../i18n";
export function formatErrorMessage(message: string): string {
  const text = message.trim();
  if (!text) return i18n.t("errors.unableToCompleteTheRequest");

  const formatted = text.charAt(0).toUpperCase() + text.slice(1);
  return /[.!?]$/.test(formatted) ? formatted : `${formatted}.`;
}
