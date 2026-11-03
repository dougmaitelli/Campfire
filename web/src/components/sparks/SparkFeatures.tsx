import { Capability } from "../../contracts/capabilities";
import { SquareCheck, SquareMinus, SquareX } from "lucide-react";
import { useTranslation, Trans } from "react-i18next";
import type { Spark } from "../../api/sparks";
import "./spark-features.css";

const featureTranslationKeys = {
  [Capability.GameMedia]: "sparkFeatures.gameMedia",
  [Capability.GameIntegration]: "sparkFeatures.gameIntegration",
  [Capability.Prune]: "sparkFeatures.resourceCleanup",
  [Capability.Configure]: "sparkFeatures.serverConfiguration",
  [Capability.Lifecycle]: "sparkFeatures.serverControls",
  [Capability.Files]: "sparkFeatures.fileManagement",
  [Capability.FileTransfer]: "sparkFeatures.fileUploadsAndDownloads",
  [Capability.FileArchive]: "sparkFeatures.archiveCreationAndExtraction",
  [Capability.Backups]: "sparkFeatures.localBackups",
  [Capability.RemoteBackups]: "sparkFeatures.remoteBackupRestore",
  [Capability.Metrics]: "sparkFeatures.resourceMonitoring",
  [Capability.Console]: "sparkFeatures.liveConsole",
  [Capability.LogAlerts]: "sparkFeatures.logAlerts",
  [Capability.DiskQuota]: "sparkFeatures.diskQuotas",
  [Capability.Transfer]: "sparkFeatures.serverTransfers",
  [Capability.Reinstall]: "sparkFeatures.serverUpdateAndReinstall",
  [Capability.SelfUpdate]: "sparkFeatures.remoteSparkUpdates",
} as const;

const featureRequirements: Record<string, readonly string[]> = {
  [Capability.Files]: [Capability.Files, Capability.FilePermissions],
  [Capability.SelfUpdate]: [Capability.SelfUpdate, Capability.SelfReinstall],
  [Capability.GameIntegration]: [
    Capability.GameIntegration,
    Capability.GameEvents,
  ],
};

export function SparkFeatures({ spark }: { spark: Spark }) {
  const { t } = useTranslation();
  const available = new Set(spark.capabilities ?? []);
  const features: {
    id: string;
    label: string;
    capabilities: readonly string[];
  }[] = Object.entries(featureTranslationKeys).map(([feature, key]) => ({
    id: feature,
    label: t(key),
    capabilities: featureRequirements[feature] ?? [feature],
  }));
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
