import { useTranslation } from "react-i18next";
import { StatusPill } from "../status/StatusPill";
import { formatVersion } from "../../utils/formatVersion";

export function SparkConnectionState({
  disabled,
  connected,
  version,
  updating = false,
}: {
  disabled: boolean;
  connected: boolean;
  version?: string | undefined;
  updating?: boolean;
}) {
  const { t } = useTranslation();
  const label = updating
    ? t("sparkConnectionState.updating")
    : disabled
      ? t("common.disabled")
      : connected
        ? version
          ? t("sparkConnectionState.connectedVersion", { version })
          : t("sparkConnectionState.connected")
        : t("sparkConnectionState.disconnected");
  return (
    <StatusPill
      tone={
        updating
          ? "pending"
          : disabled
            ? "warning"
            : connected
              ? "positive"
              : "negative"
      }
    >
      <span title={label}>
        {connected && !disabled && !updating && version
          ? t("sparkConnectionState.connectedVersion", {
              version: formatVersion(version),
            })
          : label}
      </span>
    </StatusPill>
  );
}
