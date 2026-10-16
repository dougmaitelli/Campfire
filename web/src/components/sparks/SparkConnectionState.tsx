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
      tone={disabled ? "neutral" : connected ? "positive" : "negative"}
    >
      {disabled ? "Disabled" : connected ? "Connected" : "Disconnected"}
    </StatusPill>
  );
}
