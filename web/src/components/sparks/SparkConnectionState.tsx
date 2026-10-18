import { StatusPill } from "../status/StatusPill";
import "./spark-connection-state.css";

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
    <span className="spark-connection-state">
      <StatusPill
        tone={disabled ? "warning" : connected ? "positive" : "negative"}
      >
        {disabled ? "Disabled" : connected ? "Connected" : "Disconnected"}
      </StatusPill>
      {!disabled && connected && version?.trim() && (
        <span className="spark-client-version" title="Spark client version">
          {version}
        </span>
      )}
    </span>
  );
}
