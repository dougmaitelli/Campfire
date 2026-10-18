import { StatusPill } from "../status/StatusPill";

export function SparkConnectionState({
  disabled,
  connected,
}: {
  disabled: boolean;
  connected: boolean;
}) {
  return (
    <StatusPill
      tone={disabled ? "warning" : connected ? "positive" : "negative"}
    >
      {disabled ? "Disabled" : connected ? "Connected" : "Disconnected"}
    </StatusPill>
  );
}
