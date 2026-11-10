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
}: {
  label: string;
  removeLabel: string;
  onRemove: () => void;
  children: ReactNode;
  body?: ReactNode;
}) {
  return (
    <Fieldset className="editable-list-row">
      <legend className="visually-hidden">{label}</legend>
      <div className="editable-list-row-fields">{children}</div>
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
      {body && <div className="editable-list-row-body">{body}</div>}
    </Fieldset>
  );
}
