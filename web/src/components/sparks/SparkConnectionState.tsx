import { useTranslation } from "react-i18next";
import { StatusPill } from "../status/StatusPill";

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
      {updating
        ? t("sparkConnectionState.updating")
        : disabled
          ? t("common.disabled")
          : connected
            ? version
              ? t("sparkConnectionState.connectedVersion", { version })
              : t("sparkConnectionState.connected")
            : t("sparkConnectionState.disconnected")}
    </StatusPill>
  );
}
