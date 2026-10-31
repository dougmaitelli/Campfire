import { useTranslation } from "react-i18next";
import type { BackupRetention } from "../../api/backups";
import { Fieldset } from "../forms/Fieldset";
import { FormField } from "../forms/FormField";
import { TextInput } from "../forms/TextInput";
import "./backup-retention.css";

export function BackupRetentionFields({
  mode,
  value,
  onChange,
}: {
  mode: "full" | "savedata";
  value: BackupRetention;
  onChange: (value: BackupRetention) => void;
}) {
  const { t } = useTranslation();
  return (
    <Fieldset>
      <legend>
        {t(mode === "full" ? "backupModes.full" : "backupModes.savedata")}
      </legend>
      <div className="backup-retention-grid">
        {(
          [
            ["local_keep_last", "serverDrawer.localCopiesToKeep", 1000],
            ["local_max_age_days", "serverDrawer.localMaximumAgeDays", 36500],
            ["remote_keep_last", "serverDrawer.s3CopiesToKeep", 1000],
            ["remote_max_age_days", "serverDrawer.s3MaximumAgeDays", 36500],
          ] as const
        ).map(([key, label, max]) => (
          <FormField key={key} label={t(label)}>
            <TextInput
              id={`backup-${mode}-${key}`}
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
          </FormField>
        ))}
      </div>
    </Fieldset>
  );
}
