import { Button } from "../forms/Button";
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
    <Button
      type="button"
      variant="danger"
      disabled={disabled || deleting}
      onClick={onDelete}
      loading={deleting}
      loadingLabel={"Deleting…"}
    >
      {"Delete"}
    </Button>
  );
}
