import type { ComponentProps, ReactNode } from "react";
import "./checkbox-field.css";

export function CheckboxField({
  children,
  ...props
}: Omit<ComponentProps<"input">, "type" | "children"> & {
  children: ReactNode;
}) {
  return (
    <label className="checkbox-field">
      <input {...props} type="checkbox" />
      <span>{children}</span>
    </label>
  );
}
