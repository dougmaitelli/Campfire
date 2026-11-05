import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { updateBackup, type Backup } from "../../api/backups";
import { backupDisplayName } from "../../utils/backupDisplayName";
import { ActionMenu } from "../forms/ActionMenu";
import { InputDialog } from "../layout/InputDialog";

export function BackupMetadataActions({
  serverID,
  backup,
  disabled = false,
  onSaved,
  onError,
}: {
  serverID: string;
  backup: Backup;
  disabled?: boolean;
  onSaved: (backup: Backup) => void;
  onError: (message: string | null) => void;
}) {
  const { t } = useTranslation();
  const [renaming, setRenaming] = useState(false);
  const [busy, setBusy] = useState(false);
  const pending = useRef(false);
  const label = backupDisplayName(backup);

  async function save(changes: { display_name?: string; pinned?: boolean }) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    onError(null);
    try {
      onSaved(await updateBackup(serverID, backup.name, changes));
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  async function togglePin() {
    try {
      await save({ pinned: !backup.pinned });
    } catch (cause) {
      onError(
        cause instanceof Error
          ? cause.message
          : t("backupMetadata.unableToUpdate"),
      );
    }
  }

  return (
    <>
      <ActionMenu
        label={t("backupMetadata.actions", { name: label })}
        items={[
          {
            label: t("backupMetadata.rename"),
            icon: "rename",
            disabled: disabled || busy,
            onSelect: () => setRenaming(true),
          },
          {
            label: t(
              backup.pinned ? "backupMetadata.unpin" : "backupMetadata.pin",
            ),
            icon: backup.pinned ? "unpin" : "pin",
            disabled: disabled || busy,
            onSelect: () => void togglePin(),
          },
        ]}
      />
      {renaming && (
        <InputDialog
          title={t("backupMetadata.rename")}
          label={t("backupMetadata.name")}
          description={t("backupMetadata.renameHelp")}
          submitLabel={t("common.saveChanges")}
          initialValue={label}
          onClose={() => setRenaming(false)}
          onSubmit={(display_name) => save({ display_name })}
        />
      )}
    </>
  );
}
