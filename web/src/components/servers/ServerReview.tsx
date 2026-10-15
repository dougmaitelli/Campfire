import type { ServerConfiguration } from "../../api/servers";

export function ServerReview({
  value,
  sparkName,
  emberName,
  notice = "Saving reserves these resources and ports. The server will remain Not installed.",
}: {
  value: ServerConfiguration;
  sparkName: string;
  emberName: string;
  notice?: string | undefined;
}) {
  return (
    <>
      <dl className="review-grid">
        <div>
          <dt>Name</dt>
          <dd>{value.name.trim()}</dd>
        </div>
        <div>
          <dt>Description</dt>
          <dd>{value.description.trim() || "None"}</dd>
        </div>
        <div>
          <dt>Spark</dt>
          <dd>{sparkName}</dd>
        </div>
        <div>
          <dt>Ember</dt>
          <dd>{emberName}</dd>
        </div>
        <div>
          <dt>Image</dt>
          <dd>{value.image}</dd>
        </div>
        <div>
          <dt>CPU</dt>
          <dd>{value.cpu_limit} cores</dd>
        </div>
        <div>
          <dt>Memory</dt>
          <dd>{value.memory_mb.toLocaleString()} MiB</dd>
        </div>
        <div>
          <dt>Disk</dt>
          <dd>{value.disk_mb.toLocaleString()} MiB</dd>
        </div>
        <div>
          <dt>Ports</dt>
          <dd>
            {value.ports
              .map(
                (port) =>
                  `${port.bind_ip}:${port.host_port} → ${port.container_port}/${port.protocol}`,
              )
              .join("\n")}
          </dd>
        </div>
        {Object.entries(value.variables).map(([name, content]) => (
          <div key={name}>
            <dt>{name}</dt>
            <dd>{content || "(empty)"}</dd>
          </div>
        ))}
      </dl>
      <p className="field-help">{notice}</p>
    </>
  );
}
