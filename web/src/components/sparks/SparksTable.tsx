import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import { Button } from "../forms/Button";
import type { Spark } from "../../api/sparks";
import { Table, TableRow } from "../table/Table";
import { SparkConnectionState } from "./SparkConnectionState";
import { PortGroups } from "../network/PortGroups";
import { formatDisplayValue } from "../../utils/formatDisplayValue";
import { SparkResourceReadings } from "./SparkResourceReadings";
import type { MonitoredServer } from "../../api/monitoring";
import "./sparks-table.css";

export function SparksTable({
  sparks,
  onDetails,
  loading,
  error,
  onRetry,
  onAdd,
  onUpdate,
  updating,
  servers,
  elapsed,
}: {
  sparks: Spark[];
  onDetails: (spark: Spark) => void;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  onAdd: () => void;
  onUpdate: (spark: Spark) => void;
  updating: string | null;
  servers: MonitoredServer[] | undefined;
  elapsed: number;
}) {
  const { t } = useTranslation();
  return (
    <Table
      className="sparks-table"
      empty={sparks.length === 0}
      loading={loading}
      error={error}
      resource={t("common.sparks")}
      emptyDescription={t("sparksTable.addYourFirstNodeToGetStarted")}
      emptyAction={
        <Button variant="primary" onClick={onAdd}>
          {t("sparksTable.addSpark")}
        </Button>
      }
      onRetry={onRetry}
    >
      <caption className="visually-hidden">
        {t("sparksTable.registeredSparks")}
      </caption>
      <thead>
        <tr>
          <th scope="col">{t("common.name")}</th>
          <th scope="col">{t("common.status")}</th>
          <th scope="col">{t("sparksTable.resources")}</th>
          <th scope="col">{t("sparkReview.gameAddress")}</th>
          <th scope="col">{t("sparksTable.allocations")}</th>
          <th scope="col" className="table-actions">
            <span className="visually-hidden">{t("common.actions")}</span>
          </th>
        </tr>
      </thead>
      <tbody>
        {sparks.map((spark) => (
          <TableRow key={spark.id} onOpen={() => onDetails(spark)}>
            <td>
              <button
                type="button"
                className="table-row-trigger spark-name"
                onClick={() => onDetails(spark)}
                aria-label={i18n.t("sparksTable.viewDetailsFor", {
                  sparkName: spark.name,
                })}
              >
                {spark.name}
              </button>
              <span className="row-id">{spark.id}</span>
            </td>
            <td>
              <SparkConnectionState
                disabled={spark.disabled}
                connected={spark.connected}
                version={spark.agent_version}
                updating={updating === spark.id}
              />
            </td>
            <td>
              <SparkResourceReadings
                spark={spark}
                servers={servers}
                elapsed={elapsed}
              />
            </td>
            <td>{formatDisplayValue(spark.game_address)}</td>
            <td>
              <PortGroups
                ports={spark.port_allocations.map((allocation) => ({
                  address: allocation.bind_ip,
                  port:
                    allocation.start === allocation.end
                      ? allocation.start
                      : `${allocation.start}–${allocation.end}`,
                  protocol: allocation.protocol,
                }))}
              />
            </td>
            <td className="table-actions">
              {spark.agent_update_available && (
                <Button
                  variant="primary"
                  disabled={
                    updating !== null ||
                    !spark.capabilities?.includes("spark.self-update.v1")
                  }
                  onClick={() => onUpdate(spark)}
                  title={
                    spark.capabilities?.includes(
                      i18n.t("sparksTable.sparkSelfUpdateV1"),
                    )
                      ? i18n.t("sparksTable.updateTo", {
                          sparkLatestAgent:
                            spark.latest_agent_version ??
                            i18n.t("sparksTable.theLatestRelease"),
                        })
                      : i18n.t(
                          "sparksTable.thisClientNeedsAManualUpgradeBeforeItSupportsRemote",
                        )
                  }
                  loading={updating === spark.id}
                  loadingLabel={i18n.t("sparksTable.updating")}
                >
                  {i18n.t("sparksTable.updateSpark")}
                </Button>
              )}
            </td>
          </TableRow>
        ))}
      </tbody>
    </Table>
  );
}
