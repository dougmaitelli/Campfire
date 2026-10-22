import i18n from "../../i18n";
import { useTranslation, Trans } from "react-i18next";
import { DetailsList, DetailItem } from "../layout/DetailsList";
import type { ServerConfiguration } from "../../api/servers";
import { PortGroups } from "../network/PortGroups";
import { formatDisplayValue } from "../../utils/formatDisplayValue";
import type { ReactNode } from "react";

export function ServerReview({
  value,
  name,
  sparkName,
  emberName,
  portDescriptions,
  notice = i18n.t(
    "serverReview.savingReservesTheseResourcesAndPortsTheServerWillRemain",
  ),
}: {
  value: ServerConfiguration;
  name?: ReactNode;
  sparkName: ReactNode;
  emberName: ReactNode;
  portDescriptions?: Record<string, string> | undefined;
  notice?: string | undefined;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DetailsList>
        <DetailItem label={t("common.name")}>{name ?? value.name}</DetailItem>
        <DetailItem label={t("common.description")}>
          {formatDisplayValue(value.description)}
        </DetailItem>
        <DetailItem label={t("common.spark")}>{sparkName}</DetailItem>
        <DetailItem label={t("common.ember")}>{emberName}</DetailItem>
        <DetailItem label={t("serverReview.image")}>{value.image}</DetailItem>
        {value.startup_command && (
          <DetailItem label={t("serverReview.startupCommand")}>
            {value.startup_command}
          </DetailItem>
        )}
        <DetailItem label={t("common.cpu")}>
          <Trans
            i18nKey="serverReview.cores"
            t={t}
            components={{ valueCpuLimit: <>{value.cpu_limit}</> }}
          />
        </DetailItem>
        <DetailItem label={t("common.memory")}>
          <Trans
            i18nKey="serverReview.mib"
            t={t}
            components={{
              valueMemoryMb: <>{value.memory_mb.toLocaleString()}</>,
            }}
          />
        </DetailItem>
        <DetailItem label={t("common.disk")}>
          {value.disk_mb === 0
            ? t("common.unlimited")
            : t("serverReview.mib2", {
                valueDiskMb: value.disk_mb.toLocaleString(),
              })}
        </DetailItem>
        <DetailItem label={t("common.ports")}>
          <PortGroups
            ports={value.ports.map((port) => ({
              address: port.bind_ip,
              port: port.host_port,
              targetPort: port.container_port,
              protocol: port.protocol,
              description:
                portDescriptions?.[`${port.container_port}/${port.protocol}`],
            }))}
          />
        </DetailItem>
        {Object.entries(value.variables).map(([name, content]) => (
          <DetailItem key={name} label={<>{name}</>}>
            {content || "(empty)"}
          </DetailItem>
        ))}
      </DetailsList>
      {notice && <p className="field-help">{notice}</p>}
    </>
  );
}
