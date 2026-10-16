export function formatErrorMessage(message: string): string {
  const text = message.trim();
  if (!text) return "Unable to complete the request.";

  const formatted = text.charAt(0).toUpperCase() + text.slice(1);
  return /[.!?]$/.test(formatted) ? formatted : `${formatted}.`;
}
