import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import { DetailsList, DetailItem } from "../layout/DetailsList";
import type { SparkSettings } from "../../api/sparks";
import { StatusPill } from "../status/StatusPill";
import { PortGroups } from "../network/PortGroups";
import { formatDisplayValue } from "../../utils/formatDisplayValue";
import "./spark-review.css";
import type { ReactNode } from "react";

interface SparkReviewProps {
  name: ReactNode;
  description: string;
  disabled: boolean;
  settings: SparkSettings;
  notice?: string;
  serverCreationRoles?: ReactNode;
}

export function SparkReview({
  name,
  description,
  disabled,
  settings,
  serverCreationRoles,
  notice = i18n.t("sparkReview.reviewTheseSettingsBeforeSaving"),
}: SparkReviewProps) {
  const { t } = useTranslation();
  return (
    <div className="spark-review">
      <DetailsList>
        <DetailItem label={t("common.name")}>{name}</DetailItem>
        <DetailItem
          label={t("common.description")}
          valueClassName="review-description"
        >
          {formatDisplayValue(description)}
        </DetailItem>
        <DetailItem label={t("sparkReview.availability")}>
          <StatusPill tone={disabled ? "warning" : "positive"}>
            {disabled ? t("common.disabled") : t("common.enabled")}
          </StatusPill>
        </DetailItem>
        {serverCreationRoles && (
          <DetailItem label={t("sparkReview.serverCreationAccess")}>
            {serverCreationRoles}
          </DetailItem>
        )}
        <DetailItem label={t("sparkReview.cpuLimit")}>
          {settings.cpu_limit === 0
            ? t("sparkReview.noLimit")
            : `${settings.cpu_limit} cores`}
        </DetailItem>
        <DetailItem label={t("sparkReview.memoryLimit")}>
          {settings.memory_mb === 0
            ? t("sparkReview.noLimit")
            : t("sparkReview.mib", {
                settingsMemoryMb: settings.memory_mb.toLocaleString(),
              })}
        </DetailItem>
        <DetailItem label={t("sparkReview.diskLimit")}>
          {settings.disk_mb === 0
            ? t("sparkReview.noLimit")
            : t("sparkReview.mib2", {
                settingsDiskMb: settings.disk_mb.toLocaleString(),
              })}
        </DetailItem>
        <DetailItem label={t("sparkReview.gameAddress")}>
          {formatDisplayValue(settings.game_address)}
        </DetailItem>
      </DetailsList>
      <h4>{t("sparkReview.portAllocations")}</h4>
      {settings.port_allocations.length === 0 ? (
        <p className="field-help">{t("sparkReview.noPortsAllocated")}</p>
      ) : (
        <PortGroups
          ports={settings.port_allocations.map((allocation) => ({
            address: allocation.bind_ip,
            port:
              allocation.start === allocation.end
                ? allocation.start
                : `${allocation.start}–${allocation.end}`,
            protocol: allocation.protocol,
          }))}
        />
      )}
      {notice && <p className="field-help">{notice}</p>}
    </div>
  );
}
