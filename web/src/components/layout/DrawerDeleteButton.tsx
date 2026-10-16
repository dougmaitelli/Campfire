export function DrawerDeleteButton({
  onDelete,
  disabled = false,
  deleting = false,
}: {
  onDelete?: (() => void) | undefined;
  disabled?: boolean;
  deleting?: boolean;
}) {
  if (!onDelete) return null;
  return (
    <button
      type="button"
      className="button button-danger"
      disabled={disabled || deleting}
      onClick={onDelete}
    >
      {deleting ? "Deleting…" : "Delete"}
    </button>
  );
}
