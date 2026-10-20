import i18n from "../../i18n";
export type UtilizationTone = "neutral" | "warning" | "critical";

export const utilizationThresholds = { warning: 0.8, critical: 0.9 } as const;

export function resourceUtilization(
  used: number | null,
  allocated: number,
  warningAt: number = utilizationThresholds.warning,
  criticalAt: number = utilizationThresholds.critical,
): { ratio: number | null; tone: UtilizationTone; title: string } {
  const ratio =
    used !== null &&
    Number.isFinite(used) &&
    used >= 0 &&
    Number.isFinite(allocated) &&
    allocated > 0
      ? used / allocated
      : null;
  const tone =
    ratio === null
      ? "neutral"
      : ratio >= criticalAt
        ? "critical"
        : ratio >= warningAt
          ? "warning"
          : "neutral";
  const title =
    ratio === null
      ? i18n.t("utilization.utilizationUnavailable")
      : i18n.t(
          tone === "critical"
            ? "utilization.capacityCritical"
            : tone === "warning"
              ? "utilization.capacityHigh"
              : "utilization.capacity",
          { percent: Math.round(ratio * 100) },
        );
  return { ratio, tone, title };
}
