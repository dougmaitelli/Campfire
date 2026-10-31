import { SquareCheck, SquareMinus, SquareX } from "lucide-react";
import { useTranslation, Trans } from "react-i18next";
import type { Spark } from "../../api/sparks";
import "./spark-features.css";

const featureTranslationKeys = {
  "server.game-media.v1": "sparkFeatures.gameMedia",
  "server.game-integration.v1": "sparkFeatures.gameIntegration",
  "spark.prune.v1": "sparkFeatures.resourceCleanup",
  "server.configure.v2": "sparkFeatures.serverConfiguration",
  "server.lifecycle.v1": "sparkFeatures.serverControls",
  "server.files.v1": "sparkFeatures.fileManagement",
  "server.files.transfer.v1": "sparkFeatures.fileUploadsAndDownloads",
  "server.files.archive.v1": "sparkFeatures.archiveCreationAndExtraction",
  "server.backups.v1": "sparkFeatures.localBackups",
  "server.backups.remote.v1": "sparkFeatures.remoteBackupRestore",
  "server.metrics.v1": "sparkFeatures.resourceMonitoring",
  "server.console.stream.v1": "sparkFeatures.liveConsole",
  "server.disk-quota.v1": "sparkFeatures.diskQuotas",
  "server.transfer.v1": "sparkFeatures.serverTransfers",
  "server.reinstall.v1": "sparkFeatures.serverUpdateAndReinstall",
  "spark.self-update.v1": "sparkFeatures.remoteSparkUpdates",
} as const;

const featureRequirements: Record<string, readonly string[]> = {
  "server.files.v1": ["server.files.v1", "server.files.permissions.v1"],
  "spark.self-update.v1": ["spark.self-update.v1", "spark.self-reinstall.v1"],
  "server.game-integration.v1": [
    "server.game-integration.v1",
    "server.game-events.v1",
  ],
};

export function SparkFeatures({ spark }: { spark: Spark }) {
  const { t } = useTranslation();
  const available = new Set(spark.capabilities ?? []);
  const features = Object.entries(featureTranslationKeys).map(
    ([feature, key]) => ({
      id: feature,
      label: t(key),
      capabilities: featureRequirements[feature] ?? [feature],
    }),
  );
  const knownCapabilities = new Set(
    features.flatMap((feature) => feature.capabilities),
  );
  features.push(
    ...[...available]
      .filter((feature) => !knownCapabilities.has(feature))
      .map((feature) => ({
        id: feature,
        label: feature,
        capabilities: [feature],
      })),
  );
  const supportedCount = features.filter((feature) =>
    feature.capabilities.every((capability) => available.has(capability)),
  ).length;
  return (
    <section
      className="spark-features"
      aria-label={t("sparkFeatures.sparkFeatures")}
    >
      <h3>
        <Trans
          i18nKey="sparkFeatures.features"
          t={t}
          components={{
            sparkConnectedAvailable: (
              <>
                {spark.connected
                  ? ` (${supportedCount}/${features.length})`
                  : ""}
              </>
            ),
          }}
        />
      </h3>
      <p className="field-help">
        {spark.connected
          ? t("sparkFeatures.sparkWireProtocol", {
              sparkAgentVersion:
                spark.agent_version ?? t("sparkFeatures.unknownVersion"),
              sparkProtocol: spark.protocol ?? t("common.unknown"),
            })
          : t("sparkFeatures.connectThisSparkToCheckFeatureAvailability")}
      </p>
      <ul className="spark-feature-checklist">
        {features.map(({ id, label, capabilities }) => {
          const supported = capabilities.filter((capability) =>
            available.has(capability),
          ).length;
          const state = !spark.connected
            ? "unknown"
            : supported === capabilities.length
              ? "available"
              : supported > 0
                ? "partial"
                : "unavailable";
          return (
            <li
              key={id}
              className={`spark-feature spark-feature-${state}`}
              title={capabilities.join(", ")}
            >
              {state === "available" ? (
                <SquareCheck aria-hidden="true" />
              ) : state === "unavailable" ? (
                <SquareX aria-hidden="true" />
              ) : (
                <SquareMinus aria-hidden="true" />
              )}
              <span>{label}</span>
              <span className="spark-feature-state">
                {state === "unknown"
                  ? t("common.unknown")
                  : state === "available"
                    ? t("sparkFeatures.available")
                    : state === "partial"
                      ? t("sparkFeatures.partialSupport")
                      : t("common.unavailable")}
              </span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
