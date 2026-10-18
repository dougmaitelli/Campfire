import type { ComponentProps } from "react";
import "./form-controls.css";

export function Textarea({
  className,
  rows = 5,
  ...props
}: ComponentProps<"textarea">) {
  return (
    <textarea
      {...props}
      rows={rows}
      className={["textarea", className].filter(Boolean).join(" ")}
    />
  );
}
