import type { ComponentProps } from "react";
import "./form-controls.css";

export function TextInput({ className, ...props }: ComponentProps<"input">) {
  return (
    <input
      {...props}
      className={["text-input", className].filter(Boolean).join(" ")}
    />
  );
}
