import { DetailsList, DetailItem } from "../layout/DetailsList";
import type { SparkSettings } from "../../api/sparks";
import { StatusPill } from "../status/StatusPill";

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
        <ul className="review-allocations">
          {settings.port_allocations.map((allocation, index) => (
            <li key={index}>
              <span>{allocation.bind_ip}</span>
              <span>
                {allocation.start === allocation.end
                  ? allocation.start
                  : `${allocation.start}–${allocation.end}`}{" "}
                / {allocation.protocol.toUpperCase()}
              </span>
            </li>
          ))}
        </ul>
      )}
      {notice && <p className="field-help">{notice}</p>}
    </div>
  );
}
