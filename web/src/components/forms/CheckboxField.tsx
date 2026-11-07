import type { ComponentProps, ReactNode } from "react";
import { Checkbox } from "./Checkbox";
import "./checkbox-field.css";

export function CheckboxField({
  children,
  className,
  tone = "default",
  ...props
}: Omit<ComponentProps<"input">, "type" | "children"> & {
  children: ReactNode;
  tone?: "default" | "danger";
}) {
  return (
    <label
      className={`checkbox-field${tone === "danger" ? " checkbox-field-danger" : ""}`}
    >
      <Checkbox
        {...props}
        className={[tone === "danger" && "selection-control-danger", className]
          .filter(Boolean)
          .join(" ")}
      />
      <span>{children}</span>
    </label>
  );
}
