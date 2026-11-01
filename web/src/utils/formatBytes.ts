import i18n from "../i18n";
import { EMPTY_VALUE } from "./formatDisplayValue";

const BYTES_PER_KIBIBYTE = 1024;
export const BYTES_PER_MEBIBYTE = BYTES_PER_KIBIBYTE ** 2;

export function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) return EMPTY_VALUE;
  const units = ["b", "kib", "mib", "gib", "tib", "pib"] as const;
  let amount = value;
  let index = 0;
  while (amount >= BYTES_PER_KIBIBYTE && index < units.length - 1) {
    amount /= BYTES_PER_KIBIBYTE;
    index++;
  }
  return i18n.t("dataSize.value", {
    value: amount.toLocaleString(i18n.resolvedLanguage, {
      maximumFractionDigits: 1,
    }),
    unit: i18n.t(`dataSize.${units[index]!}`),
  });
}
