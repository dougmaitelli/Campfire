export const EMPTY_VALUE = "-";

export function formatDisplayValue(
  value: string | number | null | undefined,
): string {
  if (value === null || value === undefined) return EMPTY_VALUE;
  return String(value) || EMPTY_VALUE;
}
