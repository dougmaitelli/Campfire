import type { SparkSettings } from "../../api/sparks";
import { StatusPill } from "../status/StatusPill";

interface SparkReviewProps {
  name: string;
  description: string;
  disabled: boolean;
  settings: SparkSettings;
  notice?: string;
}

export function SparkReview({
  name,
  description,
  disabled,
  settings,
  notice = "Review these settings before saving.",
}: SparkReviewProps) {
  return (
    <div className="spark-review">
      <dl>
        <div>
          <dt>Name</dt>
          <dd>{name.trim()}</dd>
        </div>
        <div>
          <dt>Description</dt>
          <dd className="review-description">{description.trim() || "None"}</dd>
        </div>
        <div>
          <dt>Availability</dt>
          <dd>
            <StatusPill status={disabled ? "disabled" : "enabled"} />
          </dd>
        </div>
        <div>
          <dt>CPU limit</dt>
          <dd>
            {settings.cpu_limit === 0
              ? "No limit"
              : `${settings.cpu_limit} cores`}
          </dd>
        </div>
        <div>
          <dt>Memory limit</dt>
          <dd>
            {settings.memory_mb === 0
              ? "No limit"
              : `${settings.memory_mb.toLocaleString()} MiB`}
          </dd>
        </div>
        <div>
          <dt>Disk limit</dt>
          <dd>
            {settings.disk_mb === 0
              ? "No limit"
              : `${settings.disk_mb.toLocaleString()} MiB`}
          </dd>
        </div>
        <div>
          <dt>Game address</dt>
          <dd>{settings.game_address.trim() || "Not set"}</dd>
        </div>
      </dl>
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
