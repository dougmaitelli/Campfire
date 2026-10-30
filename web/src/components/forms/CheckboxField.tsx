import type { ComponentProps, ReactNode } from "react";
import "./checkbox-field.css";
import "./selection-control.css";

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
      <input
        {...props}
        className={[
          "selection-control",
          tone === "danger" && "selection-control-danger",
          className,
        ]
          .filter(Boolean)
          .join(" ")}
        type="checkbox"
      />
      <span>{children}</span>
    </label>
  );
}
