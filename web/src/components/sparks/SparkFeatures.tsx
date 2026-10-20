import i18n from "../../i18n";
import { useTranslation, Trans } from "react-i18next";
import type { Spark } from "../../api/sparks";
import "./spark-features.css";

const featureLabels: Record<string, string> = {
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
