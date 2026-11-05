import type { Spark } from "../api/sparks";

export function isEnabledSpark(spark: Spark): boolean {
  return !spark.disabled;
}
