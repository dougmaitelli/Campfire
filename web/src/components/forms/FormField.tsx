import {
  cloneElement,
  useId,
  type ReactElement,
  type ReactNode,
  type AriaAttributes,
} from "react";
import "./form-field.css";

type FieldControlProps = AriaAttributes & { id?: string };

export function FormField({
  label,
  labelAction,
  description,
  error,
  children,
  className,
}: {
  label: ReactNode;
  labelAction?: ReactNode;
  description?: ReactNode;
  error?: ReactNode;
  children: ReactElement<FieldControlProps>;
  className?: string;
}) {
  const generatedId = useId();
  const id = children.props.id ?? generatedId;
  const describedBy = [
    children.props["aria-describedby"],
    description && `${id}-help`,
    error && `${id}-error`,
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <div className={["form-field", className].filter(Boolean).join(" ")}>
      {labelAction ? (
        <div className="form-field-label-row">
          <label htmlFor={id}>{label}</label>
          {labelAction}
        </div>
      ) : (
        <label htmlFor={id}>{label}</label>
      )}
      {cloneElement(children, {
        id,
        "aria-describedby": describedBy || undefined,
        "aria-invalid": error ? true : children.props["aria-invalid"],
      })}
      {description && (
        <p className="field-help" id={`${id}-help`}>
          {description}
        </p>
      )}
      {error && (
        <p className="form-field-error" id={`${id}-error`} role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
