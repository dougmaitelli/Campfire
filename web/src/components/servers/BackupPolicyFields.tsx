import { Select } from "../forms/Select";
import type { BackupPolicy } from "../../api/backups";

export function BackupPolicyFields({
  value,
  s3Available,
  onChange,
}: {
  value: BackupPolicy;
  s3Available: boolean;
  onChange: (value: BackupPolicy) => void;
}) {
  return (
    <>
      <label htmlFor="backup-storage">Background S3 sync</label>
      <Select
        id="backup-storage"
        value={value.storage}
        onChange={(event) =>
          onChange({
            ...value,
            storage: event.target.value as BackupPolicy["storage"],
          })
        }
      >
        <option value="local">Disabled</option>
        <option value="s3" disabled={!s3Available}>
          Enabled
        </option>
      </Select>
      {!s3Available && (
        <p className="field-help">
          Configure S3 on Campfire before enabling sync.
        </p>
      )}
      {(
        [
          ["local_keep_last", "Local copies to keep", 1000],
          ["local_max_age_days", "Local maximum age (days)", 36500],
          ["remote_keep_last", "S3 copies to keep", 1000],
          ["remote_max_age_days", "S3 maximum age (days)", 36500],
        ] as const
      ).map(([key, label, max]) => (
        <div key={key}>
          <label htmlFor={`backup-${key}`}>{label}</label>
          <input
            id={`backup-${key}`}
            type="number"
            min={0}
            max={max}
            step={1}
            required
            value={value[key]}
            onChange={(event) =>
              onChange({ ...value, [key]: Number(event.target.value) })
            }
          />
        </div>
      ))}
      <p className="field-help">
        Zero disables a limit. Local and S3 copies expire independently. A local
        copy awaiting a required S3 upload is retained until sync succeeds.
      </p>
    </>
  );
}
