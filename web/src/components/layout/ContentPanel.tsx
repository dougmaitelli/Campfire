import type { ComponentProps } from "react";
import "./content-panel.css";

export function ContentPanel({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      {...props}
      className={["content-panel", className].filter(Boolean).join(" ")}
    />
  );
}
