import type { ReactNode } from "react";
import { Fieldset } from "./Fieldset";
import { Button } from "./Button";
import { NavigationIcon } from "../navigation/NavigationIcon";
import "./allocation-row.css";

export function AllocationRow({
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
    <Fieldset className="allocation-row">
      <legend className="visually-hidden">{label}</legend>
      <div className="allocation-row-fields">{children}</div>
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
      {body && <div className="allocation-row-body">{body}</div>}
    </Fieldset>
  );
}
