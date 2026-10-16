import type { Spark } from "../../api/sparks";

const featureLabels: Record<string, string> = {
  "server.configure.v1": "Server configuration",
  "server.ember.behavior.v1": "Extended Ember behavior",
  "server.lifecycle.v1": "Server controls",
  "server.files.v1": "File management",
  "server.backups.v1": "Local backups",
  "server.backups.delete.v1": "Backup deletion",
  "server.backups.remote.v1": "S3 backup restoration",
  "server.metrics.v1": "Usage monitoring",
  "server.console.stream.v1": "Live console",
  "server.disk-quota.v1": "Disk quotas",
  "server.transfer.v1": "Server transfers",
  "server.upgrade.v1": "Updates and reinstalls",
};

export function SparkFeatures({ spark }: { spark: Spark }) {
  return (
    <section aria-label="Spark features">
      <h3>
        Features{spark.connected ? ` (${spark.capabilities?.length ?? 0})` : ""}
      </h3>
      {spark.connected ? (
        <>
          <p className="field-help">
            Spark {spark.agent_version ?? "unknown version"} · Wire protocol{" "}
            {spark.protocol ?? "unknown"}
          </p>
          {spark.capabilities?.length ? (
            <ul>
              {spark.capabilities.map((feature) => (
                <li key={feature} title={feature}>
                  {featureLabels[feature] ?? feature}
                </li>
              ))}
            </ul>
          ) : (
            <p className="field-help">
              No features negotiated. Update this Spark to enable operations.
            </p>
          )}
        </>
      ) : (
        <p className="field-help">
          Connect this Spark to view its version and supported features.
        </p>
      )}
    </section>
  );
}
