import type { Spark } from "../../api/sparks";
import "./spark-features.css";

const featureLabels: Record<string, string> = {
  "server.configure.v1": "Server configuration",
  "server.ember.behavior.v1": "Extended Ember behavior",
  "server.lifecycle.v1": "Server controls",
  "server.files.v1": "File management",
  "server.files.transfer.v1": "File uploads and downloads",
  "server.files.archive.v1": "Archive creation and extraction",
  "server.backups.v1": "Local backups",
  "server.backups.delete.v1": "Backup deletion",
  "server.backups.remote.v1": "S3 backup restoration",
  "server.metrics.v1": "Usage monitoring",
  "server.console.stream.v1": "Live console",
  "server.disk-quota.v1": "Disk quotas",
  "server.transfer.v1": "Server transfers",
  "server.upgrade.v1": "Updates and reinstalls",
  "server.reinstall.clean.v1": "Clean reinstalls",
  "spark.self-update.v1": "Remote Spark updates",
};

export function SparkFeatures({ spark }: { spark: Spark }) {
  const available = new Set(spark.capabilities ?? []);
  const features = [
    ...Object.entries(featureLabels),
    ...(spark.capabilities ?? [])
      .filter((feature) => !(feature in featureLabels))
      .map((feature): [string, string] => [feature, feature]),
  ];
  return (
    <section className="spark-features" aria-label="Spark features">
      <h3>
        Features
        {spark.connected ? ` (${available.size}/${features.length})` : ""}
      </h3>
      <p className="field-help">
        {spark.connected
          ? `Spark ${spark.agent_version ?? "unknown version"} · Wire protocol ${spark.protocol ?? "unknown"}`
          : "Connect this Spark to check feature availability."}
      </p>
      <ul className="spark-feature-checklist">
        {features.map(([feature, label]) => {
          const supported = spark.connected && available.has(feature);
          const state = !spark.connected
            ? "unknown"
            : supported
              ? "available"
              : "unavailable";
          return (
            <li
              key={feature}
              className={`spark-feature spark-feature-${state}`}
              title={feature}
            >
              <svg
                viewBox="0 0 20 20"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                aria-hidden="true"
              >
                <rect x="2" y="2" width="16" height="16" rx="4" />
                {supported ? (
                  <path d="m6 10 3 3 5-6" />
                ) : spark.connected ? (
                  <path d="m7 7 6 6m-6 0 6-6" />
                ) : (
                  <path d="M6 10h8" />
                )}
              </svg>
              <span>{label}</span>
              <span className="spark-feature-state">
                {!spark.connected
                  ? "Unknown"
                  : supported
                    ? "Available"
                    : "Unavailable"}
              </span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
