import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { deleteArchive, type CatalogBackup } from "../../api/backups";
import { backupDisplayName } from "../../utils/backupDisplayName";
import { Button } from "../forms/Button";
import { ConfirmDialog } from "../layout/ConfirmDialog";
import { NavigationIcon } from "../navigation/NavigationIcon";

export function ArchiveDelete({
  backup,
  onDeleted,
}: {
  backup: CatalogBackup;
  onDeleted: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  async function remove() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await deleteArchive(backup.id);
      setOpen(false);
      onDeleted();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : t("backupLibrary.deleteFailed"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <>
      <Button
        variant="danger"
        iconOnly
        disabled={backup.pinned || busy}
        onClick={() => setOpen(true)}
        aria-label={t("backupLibrary.deleteLabel", {
          name: backupDisplayName(backup),
        })}
        title={t(
          backup.pinned
            ? "backupLibrary.unpinFirst"
            : "backupLibrary.deleteTitle",
        )}
      >
        <NavigationIcon name="delete" />
      </Button>
      {open && (
        <ConfirmDialog
          title={t("backupLibrary.deleteTitle")}
          description={t("backupLibrary.deleteDescription", {
            name: backupDisplayName(backup),
          })}
          confirmLabel={t("common.delete")}
          danger
          busy={busy}
          error={error}
          onClose={() => setOpen(false)}
          onConfirm={() => void remove()}
        />
      )}
    </>
  );
}
