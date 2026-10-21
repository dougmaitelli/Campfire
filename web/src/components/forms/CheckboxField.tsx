import type { ComponentProps, ReactNode } from "react";
import "./checkbox-field.css";
import "./selection-control.css";

export function CheckboxField({
  children,
  className,
  ...props
}: Omit<ComponentProps<"input">, "type" | "children"> & {
  children: ReactNode;
}) {
  return (
    <label className="checkbox-field">
      <input
        {...props}
        className={["selection-control", className].filter(Boolean).join(" ")}
        type="checkbox"
      />
      <span>{children}</span>
    </label>
  );
}
