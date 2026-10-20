export function cpuUsageDivisor(cores: number): number {
  return cores > 0 ? cores : 100;
}

export function formatCPUUsage(value: number, cores: number): string {
  return cores > 0
    ? `${value.toLocaleString(undefined, { maximumFractionDigits: 1 })}%`
    : `${value.toLocaleString(undefined, { maximumFractionDigits: 2 })} cores`;
}
