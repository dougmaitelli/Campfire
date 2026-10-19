import type { ComponentProps } from "react";
import "./fieldset.css";

export function Fieldset({ className, ...props }: ComponentProps<"fieldset">) {
  return (
    <fieldset
      {...props}
      className={["fieldset", className].filter(Boolean).join(" ")}
    />
  );
}
