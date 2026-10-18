import { StatusPill } from "../status/StatusPill";
import "./port-pill.css";

export function PortPill({
  address,
  port,
  targetPort,
  protocol,
}: {
  address?: string | undefined;
  port: number | string;
  targetPort?: number;
  protocol: string;
}) {
  const host =
    address?.includes(":") && !address.startsWith("[")
      ? `[${address}]`
      : address;
  return (
    <span className="port-pill">
      <StatusPill indicator={false}>
        {host ? `${host}:` : ""}
        {port}
        {targetPort !== undefined ? ` → ${targetPort}` : ""} /{" "}
        {protocol.toUpperCase()}
      </StatusPill>
    </span>
  );
}
