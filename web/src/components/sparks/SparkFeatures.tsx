import i18n from "../../i18n";
import { SquareCheck, SquareMinus, SquareX } from "lucide-react";
import { useTranslation, Trans } from "react-i18next";
import type { Spark } from "../../api/sparks";
import "./spark-features.css";

const featureLabels: Record<string, string> = {
  get "server.game-integration.v1"() {
    return i18n.t("sparkFeatures.gameIntegration");
  },
  get "spark.prune.v1"() {
    return i18n.t("sparkFeatures.resourceCleanup");
  },
  get "server.configure.v1"() {
    return i18n.t("sparkFeatures.serverConfiguration");
  },
  get "server.lifecycle.v1"() {
    return i18n.t("sparkFeatures.serverControls");
  },
  get "server.files.v1"() {
    return i18n.t("sparkFeatures.fileManagement");
  },
  get "server.files.transfer.v1"() {
    return i18n.t("sparkFeatures.fileUploadsAndDownloads");
  },
  get "server.files.archive.v1"() {
    return i18n.t("sparkFeatures.archiveCreationAndExtraction");
  },
  get "server.backups.v1"() {
    return i18n.t("sparkFeatures.localBackups");
  },
  get "server.backups.remote.v1"() {
    return i18n.t("sparkFeatures.remoteBackups");
  },
  get "server.metrics.v1"() {
    return i18n.t("sparkFeatures.usageMonitoring");
  },
  get "server.console.stream.v1"() {
    return i18n.t("sparkFeatures.liveConsole");
  },
  get "server.disk-quota.v1"() {
    return i18n.t("sparkFeatures.diskQuotas");
  },
  get "server.transfer.v1"() {
    return i18n.t("sparkFeatures.serverTransfers");
  },
  get "server.reinstall.v1"() {
    return i18n.t("common.reinstall");
  },
  get "spark.self-update.v1"() {
    return i18n.t("sparkFeatures.remoteSparkUpdates");
  },
};

export function SparkFeatures({ spark }: { spark: Spark }) {
  const { t } = useTranslation();
  const available = new Set(spark.capabilities ?? []);
  const features = [
    ...Object.entries(featureLabels),
    ...(spark.capabilities ?? [])
      .filter((feature) => !(feature in featureLabels))
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
                  ? i18n.t("common.unknown")
                  : supported
                    ? i18n.t("sparkFeatures.available")
                    : i18n.t("common.unavailable")}
              </span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
