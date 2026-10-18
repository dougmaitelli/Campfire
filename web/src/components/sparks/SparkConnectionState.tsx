import { StatusPill } from "../status/StatusPill";

export function SparkConnectionState({
  disabled,
  connected,
  version,
}: {
  disabled: boolean;
  connected: boolean;
  version?: string | undefined;
}) {
  return (
    <StatusPill
      tone={disabled ? "warning" : connected ? "positive" : "negative"}
    >
      {disabled
        ? "Disabled"
        : connected
          ? `Connected${version?.trim() ? ` - ${version.trim()}` : ""}`
          : "Disconnected"}
    </StatusPill>
  );
}
