import { DetailsList, DetailItem } from "../layout/DetailsList";
import type { SparkSettings } from "../../api/sparks";
import { StatusPill } from "../status/StatusPill";
import { PortPill } from "../network/PortPill";

interface SparkReviewProps {
  name: string;
  description: string;
  disabled: boolean;
  settings: SparkSettings;
  notice?: string;
  serverCreationRoles?: string;
}

export function SparkReview({
  name,
  description,
  disabled,
  settings,
  serverCreationRoles,
  notice = "Review these settings before saving.",
}: SparkReviewProps) {
  return (
    <div className="spark-review">
      <DetailsList>
        <DetailItem label="Name">{name.trim()}</DetailItem>
        <DetailItem label="Description" valueClassName="review-description">
          {description.trim() || "None"}
        </DetailItem>
        <DetailItem label="Availability">
          <StatusPill tone={disabled ? "warning" : "positive"}>
            {disabled ? "Disabled" : "Enabled"}
          </StatusPill>
        </DetailItem>
        {serverCreationRoles && (
          <DetailItem label="Server creation access">
            {serverCreationRoles}
          </DetailItem>
        )}
        <DetailItem label="CPU limit">
          {settings.cpu_limit === 0
            ? "No limit"
            : `${settings.cpu_limit} cores`}
        </DetailItem>
        <DetailItem label="Memory limit">
          {settings.memory_mb === 0
            ? "No limit"
            : `${settings.memory_mb.toLocaleString()} MiB`}
        </DetailItem>
        <DetailItem label="Disk limit">
          {settings.disk_mb === 0
            ? "No limit"
            : `${settings.disk_mb.toLocaleString()} MiB`}
        </DetailItem>
        <DetailItem label="Game address">
          {settings.game_address.trim() || "Not set"}
        </DetailItem>
      </DetailsList>
      <h4>Port allocations</h4>
      {settings.port_allocations.length === 0 ? (
        <p className="field-help">No ports allocated.</p>
      ) : (
        <div className="pill-group">
          {settings.port_allocations.map((allocation, index) => (
            <PortPill
              key={index}
              address={allocation.bind_ip}
              port={
                allocation.start === allocation.end
                  ? allocation.start
                  : `${allocation.start}–${allocation.end}`
              }
              protocol={allocation.protocol}
            />
          ))}
        </div>
      )}
      {notice && <p className="field-help">{notice}</p>}
    </div>
  );
}
