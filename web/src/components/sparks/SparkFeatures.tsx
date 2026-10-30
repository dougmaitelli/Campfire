import { SquareCheck, SquareMinus, SquareX } from "lucide-react";
import { useTranslation, Trans } from "react-i18next";
import type { Spark } from "../../api/sparks";
import "./spark-features.css";

const featureTranslationKeys = {
  "server.files.permissions.v1": "serverFilesDrawer.permissions",
  "server.startup-override.v1": "serverDrawer.customStartupCommand",
  "server.game-media.v1": "sparkFeatures.gameMedia",
  "server.game-events.v1": "sparkFeatures.gameEvents",
  "server.game-integration.v1": "sparkFeatures.gameIntegration",
  "spark.prune.v1": "sparkFeatures.resourceCleanup",
  "server.configure.v1": "sparkFeatures.serverConfiguration",
  "server.lifecycle.v1": "sparkFeatures.serverControls",
  "server.files.v1": "sparkFeatures.fileManagement",
  "server.files.transfer.v1": "sparkFeatures.fileUploadsAndDownloads",
  "server.files.archive.v1": "sparkFeatures.archiveCreationAndExtraction",
  "server.backups.v1": "sparkFeatures.localBackups",
  "server.backups.remote.v1": "sparkFeatures.remoteBackups",
  "server.metrics.v1": "sparkFeatures.usageMonitoring",
  "server.console.stream.v1": "sparkFeatures.liveConsole",
  "server.disk-quota.v1": "sparkFeatures.diskQuotas",
  "server.transfer.v1": "sparkFeatures.serverTransfers",
  "server.reinstall.v1": "common.reinstall",
  "spark.self-update.v1": "sparkFeatures.remoteSparkUpdates",
  "spark.self-reinstall.v1": "sparkFeatures.remoteSparkReinstall",
} as const;

export function SparkFeatures({ spark }: { spark: Spark }) {
  const { t } = useTranslation();
  const available = new Set(spark.capabilities ?? []);
  const features = [
    ...Object.entries(featureTranslationKeys).map(
      ([feature, key]): [string, string] => [feature, t(key)],
    ),
    ...(spark.capabilities ?? [])
      .filter((feature) => !Object.hasOwn(featureTranslationKeys, feature))
      .map((feature): [string, string] => [feature, feature]),
  ];
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
                  ? ` (${available.size}/${features.length})`
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
              {supported ? (
                <SquareCheck aria-hidden="true" />
              ) : spark.connected ? (
                <SquareX aria-hidden="true" />
              ) : (
                <SquareMinus aria-hidden="true" />
              )}
              <span>{label}</span>
              <span className="spark-feature-state">
                {!spark.connected
                  ? t("common.unknown")
                  : supported
                    ? t("sparkFeatures.available")
                    : t("common.unavailable")}
              </span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
