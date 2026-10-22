import "./port-pill.css";

export function PortPill({
  port,
  protocol,
  description,
}: {
  port: number | string;
  protocol: string;
  description?: string | undefined;
}) {
  return (
    <span className="port-pill">
      {port} {protocol.toUpperCase()}
      {description && (
        <span className="port-pill-description">({description})</span>
      )}
    </span>
  );
}
