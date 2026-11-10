import i18n from "../../i18n";
import { useTranslation, Trans } from "react-i18next";
import { DetailsList, DetailItem } from "../layout/DetailsList";
import type { ServerConfiguration } from "../../api/servers";
import { PortGroups } from "../network/PortGroups";
import { formatDisplayValue } from "../../utils/formatDisplayValue";
import { formatBytes, BYTES_PER_MEBIBYTE } from "../../utils/formatBytes";
import type { ReactNode } from "react";
import { ToggleSwitch } from "../forms/ToggleSwitch";
import type { ServerConfigurationChanges } from "../../utils/serverConfigurationChanges";

export function ServerReview({
  value,
  name,
  sparkName,
  emberName,
  portDescriptions,
  variableTypes,
  changes,
  notice = i18n.t(
    "serverReview.savingReservesTheseResourcesAndPortsTheServerWillRemain",
  ),
}: {
  value: ServerConfiguration;
  name?: ReactNode;
  sparkName: ReactNode;
  emberName: ReactNode;
  portDescriptions?: Record<string, string> | undefined;
  variableTypes?: Record<string, string> | undefined;
  changes?: ServerConfigurationChanges | undefined;
  notice?: string | undefined;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DetailsList>
        <DetailItem label={t("common.name")} change={changes?.fields.name}>
          {name ?? value.name}
        </DetailItem>
        <DetailItem
          label={t("common.description")}
          change={changes?.fields.description}
        >
          {formatDisplayValue(value.description)}
        </DetailItem>
        <DetailItem label={t("common.spark")} change={changes?.fields.spark_id}>
          {sparkName}
        </DetailItem>
        <DetailItem label={t("common.ember")} change={changes?.fields.ember_id}>
          {emberName}
        </DetailItem>
        <DetailItem
          label={t("serverReview.image")}
          change={changes?.fields.image}
        >
          {value.image}
        </DetailItem>
        <DetailItem
          label={t("serverDrawer.gameIntegration")}
          change={changes?.fields.game_integration}
        >
          <ToggleSwitch
            aria-label={t("serverDrawer.gameIntegration")}
            checked={value.game_integration ?? false}
            disabled
          />
        </DetailItem>
        {(!!value.startup_command || !!changes?.fields.startup_command) && (
          <DetailItem
            label={t("serverReview.startupCommand")}
            change={changes?.fields.startup_command}
          >
            {formatDisplayValue(value.startup_command)}
          </DetailItem>
        )}
        {(!!value.startup_override || !!changes?.fields.startup_override) && (
          <DetailItem
            label={t("serverDrawer.customStartupCommand")}
            change={changes?.fields.startup_override}
          >
            <code>
              {value.startup_override?.length
                ? value.startup_override
                : t("serverDrawer.resetStartupCommand")}
            </code>
          </DetailItem>
        )}
        <DetailItem label={t("common.cpu")} change={changes?.fields.cpu_limit}>
          <Trans
            i18nKey="serverReview.cores"
            t={t}
            components={{ valueCpuLimit: <>{value.cpu_limit}</> }}
          />
        </DetailItem>
        <DetailItem
          label={t("common.memory")}
          change={changes?.fields.memory_mb}
        >
          {formatBytes(value.memory_mb * BYTES_PER_MEBIBYTE)}
        </DetailItem>
        <DetailItem label={t("common.disk")} change={changes?.fields.disk_mb}>
          {value.disk_mb === 0
            ? t("common.unlimited")
            : formatBytes(value.disk_mb * BYTES_PER_MEBIBYTE)}
        </DetailItem>
        <DetailItem label={t("common.ports")} change={changes?.fields.ports}>
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
          <DetailItem
            key={name}
            label={<>{name}</>}
            change={changes?.variables[name]}
          >
            {variableTypes?.[name] === "boolean" &&
            (content === "0" || content === "1") ? (
              <ToggleSwitch
                aria-label={name}
                checked={content === "1"}
                disabled
              />
            ) : (
              content || "(empty)"
            )}
          </DetailItem>
        ))}
      </DetailsList>
      {notice && <p className="field-help">{notice}</p>}
    </>
  );
}
