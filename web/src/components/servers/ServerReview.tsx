import { DetailsList, DetailItem } from "../layout/DetailsList";
import type { ServerConfiguration } from "../../api/servers";
import { PortPill } from "../network/PortPill";

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
      <DetailsList>
        <DetailItem label="Name">{value.name.trim()}</DetailItem>
        <DetailItem label="Description">
          {value.description.trim() || "None"}
        </DetailItem>
        <DetailItem label="Spark">{sparkName}</DetailItem>
        <DetailItem label="Ember">{emberName}</DetailItem>
        <DetailItem label="Image">{value.image}</DetailItem>
        <DetailItem label="CPU">{value.cpu_limit} cores</DetailItem>
        <DetailItem label="Memory">
          {value.memory_mb.toLocaleString()} MiB
        </DetailItem>
        <DetailItem label="Disk">
          {value.disk_mb === 0
            ? "Unlimited"
            : `${value.disk_mb.toLocaleString()} MiB`}
        </DetailItem>
        <DetailItem label="Ports">
          <div className="pill-group">
            {value.ports.map((port, index) => (
              <PortPill
                key={index}
                address={port.bind_ip}
                port={port.host_port}
                targetPort={port.container_port}
                protocol={port.protocol}
              />
            ))}
          </div>
        </DetailItem>
        {Object.entries(value.variables).map(([name, content]) => (
          <DetailItem key={name} label={<>{name}</>}>
            {content || "(empty)"}
          </DetailItem>
        ))}
      </DetailsList>
      {notice && <p className="field-help">{notice}</p>}
    </>
  );
}
