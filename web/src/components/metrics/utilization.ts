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
      ? "Utilization unavailable"
      : `${Math.round(ratio * 100)}% of allocated capacity${tone === "critical" ? " · Critical utilization" : tone === "warning" ? " · High utilization" : ""}`;
  return { ratio, tone, title };
}
