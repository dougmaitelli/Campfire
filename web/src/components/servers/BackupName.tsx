import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { Backup } from "../../api/backups";
import { BackupMode } from "../../contracts/domains";
import { backupDisplayName } from "../../utils/backupDisplayName";
import { NavigationIcon } from "../navigation/NavigationIcon";
import "./backup-name.css";

export function BackupName({
  backup,
  children,
}: {
  backup: Backup;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <>
      <span className="backup-name">
        {children ?? backupDisplayName(backup)}
        {backup.pinned && (
          <span
            role="img"
            aria-label={t("backupMetadata.pinned")}
            title={t("backupMetadata.pinHelp")}
          >
            <NavigationIcon name="pin" />
          </span>
        )}
      </span>
      <span className="row-description">
        {t(
          backup.mode === BackupMode.Savedata
            ? "backupModes.savedata"
            : "backupModes.full",
        )}
        {backup.display_name && <> · {backup.name}</>}
      </span>
    </>
  );
}
