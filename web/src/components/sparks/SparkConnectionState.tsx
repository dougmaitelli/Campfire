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
        ? "Updating…"
        : disabled
          ? "Disabled"
          : connected
            ? `Connected${version?.trim() ? ` - ${version.trim()}` : ""}`
            : "Disconnected"}
    </StatusPill>
  );
}
