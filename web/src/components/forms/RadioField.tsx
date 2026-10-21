import type { ComponentProps, ReactNode } from "react";
import "./selection-control.css";
import "./radio-field.css";

export function RadioField({
  children,
  description,
  tone = "default",
  className,
  ...props
}: Omit<ComponentProps<"input">, "type" | "children"> & {
  children: ReactNode;
  description?: ReactNode;
  tone?: "default" | "danger";
}) {
  return (
    <label
      className={`radio-field${tone === "danger" ? " radio-field-danger" : ""}`}
    >
      <input
        {...props}
        type="radio"
        className={[
          "selection-control",
          tone === "danger" && "selection-control-danger",
          className,
        ]
          .filter(Boolean)
          .join(" ")}
      />
      <span className="radio-field-copy">
        <span className="radio-field-label">{children}</span>
        {description && (
          <span className="radio-field-description">{description}</span>
        )}
      </span>
    </label>
  );
}
