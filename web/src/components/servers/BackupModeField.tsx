import { useTranslation } from "react-i18next";
import type { BackupMode } from "../../api/backups";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";

export function BackupModeField({
  value,
  onChange,
  available,
  disabled = false,
}: {
  value: BackupMode;
  onChange: (value: BackupMode) => void;
  available: boolean;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <FormField
      label={t("backupModes.label")}
      description={!available ? t("backupModes.unavailable") : undefined}
    >
      <Select
        value={value}
        disabled={disabled}
        onValueChange={(selected) => onChange(selected as BackupMode)}
      >
        <option value="full">{t("backupModes.full")}</option>
        <option value="savedata" disabled={!available}>
          {t("backupModes.savedata")}
        </option>
      </Select>
    </FormField>
  );
}
