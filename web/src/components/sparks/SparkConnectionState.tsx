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
      status={disabled ? "disabled" : connected ? "connected" : "disconnected"}
    >
      {disabled ? "Disabled" : connected ? "Connected" : "Disconnected"}
    </StatusPill>
  );
}
