import type { ReactNode } from "react";
import { Fieldset } from "./Fieldset";
import { Button } from "./Button";
import { NavigationIcon } from "../navigation/NavigationIcon";
import "./editable-list-row.css";

export function EditableListRow({
  label,
  removeLabel,
  onRemove,
  children,
  body,
  reorderControls,
}: {
  label: string;
  removeLabel: string;
  onRemove: () => void;
  children: ReactNode;
  body?: ReactNode;
  reorderControls?: ReactNode;
}) {
  const remove = (
    <Button
      type="button"
      variant="danger"
      iconOnly
      aria-label={removeLabel}
      title={removeLabel}
      onClick={onRemove}
    >
      <NavigationIcon name="close" />
    </Button>
  );
  return (
    <Fieldset
      className={
        reorderControls
          ? "editable-list-row editable-list-row-reorderable"
          : "editable-list-row"
      }
    >
      <legend className="visually-hidden">{label}</legend>
      <div className="editable-list-row-fields">{children}</div>
      {reorderControls ? (
        <div className="editable-list-row-actions">
          {reorderControls}
          {remove}
        </div>
      ) : (
        remove
      )}
      {body && <div className="editable-list-row-body">{body}</div>}
    </Fieldset>
  );
}
