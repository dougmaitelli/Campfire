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
      {port} {protocol.toUpperCase()}
    </span>
  );
}
