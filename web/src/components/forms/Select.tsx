import type { ComponentProps } from "react";
import "./select.css";

export function Select({ className, ...props }: ComponentProps<"select">) {
  return (
    <select
      {...props}
      className={["select", className].filter(Boolean).join(" ")}
    />
  );
}
