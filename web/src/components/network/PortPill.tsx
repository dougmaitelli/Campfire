import { StatusPill } from "../status/StatusPill";
import "./port-pill.css";

export function PortPill({
  port,
  protocol,
}: {
  port: number | string;
  protocol: string;
}) {
  return (
    <span className="port-pill">
      <StatusPill indicator={false}>
        {port} {protocol.toUpperCase()}
      </StatusPill>
    </span>
  );
}
